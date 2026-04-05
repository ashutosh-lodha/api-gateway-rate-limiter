package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// -------------------------------------------------------------------
// REDIS SETUP
// -------------------------------------------------------------------

var ctx = context.Background()

var rdb *redis.Client

// -------------------------------------------------------------------
// MYSQL CONNECTION
// -------------------------------------------------------------------

var db *sql.DB

// -------------------------------------------------------------------
// CONFIGURATION
// -------------------------------------------------------------------

var requestLogStream = "request_logs_stream"

var requestLimit int64 = 5

var endpointLimits = map[string]int64{
	"/login":  3,
	"/orders": 10,
}

var totalRequests = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of requests",
	},
)

var rateLimitedRequests = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "rate_limited_requests_total",
		Help: "Total number of rate limited requests",
	},
)

// -------------------------------------------------------------------
// CORS MIDDLEWARE
// -------------------------------------------------------------------

func enableCORS(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == "OPTIONS" {
			return
		}

		next(w, r)
	}
}

// -------------------------------------------------------------------
// USER LIMIT FETCHER BASED ON FREE/PRO PLAN
// -------------------------------------------------------------------

func getUserLimit(apiKey string) int64 {

	var limit int64

	query := `SELECT request_limit FROM api_keys WHERE api_key = ?`

	err := db.QueryRow(query, apiKey).Scan(&limit)

	if err != nil {
		// fallback if key not found
		return requestLimit
	}

	return limit
}

// -------------------------------------------------------------------
// RATE LIMITING MIDDLEWARE
// -------------------------------------------------------------------

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		apiKey := r.Header.Get("x-api-key")
		totalRequests.Inc()

		if apiKey == "" {
			http.Error(w, "API key missing", http.StatusBadRequest)
			return
		}

		endpoint := r.URL.Path

		limit := getUserLimit(apiKey)

		if endpointLimit, ok := endpointLimits[endpoint]; ok {
			if endpointLimit < limit {
				limit = endpointLimit
			}
		}

		key := "rate_limit:" + apiKey + ":" + endpoint

		// Use Redis INCR with expiration for atomicity
		/*
			count, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				http.Error(w, "Redis error", http.StatusInternalServerError)
				return
			}

			if count == 1 {
				rdb.Expire(ctx, key, 60*time.Second)
			}
		*/

		//---------------------------------------------------------------------------------
		// Instead we will use sliding window approach with sorted sets for better accuracy
		now := time.Now().Unix()

		window := int64(60) // 60 seconds window

		// Remove old requests
		rdb.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-window))

		// Add current request
		rdb.ZAdd(ctx, key, redis.Z{
			Score:  float64(now),
			Member: fmt.Sprintf("%d", now),
		})

		// Count requests in window
		count, err := rdb.ZCard(ctx, key).Result()
		if err != nil {
			http.Error(w, "Redis error", http.StatusInternalServerError)
			return
		}

		// Set expiry (optional cleanup)
		rdb.Expire(ctx, key, time.Duration(window)*time.Second)
		//---------------------------------------------------------------------------------

		remaining := limit - count
		if remaining < 0 {
			remaining = 0
		}

		status := "allowed"
		if count > limit {
			status = "blocked"
		}

		fmt.Println("Request:",
			"API Key:", apiKey,
			"Endpoint:", endpoint,
			"Count:", count,
			"Limit:", limit,
			"Status:", status,
		)

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		ttl, err := rdb.TTL(ctx, key).Result()
		if err == nil {
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", int(ttl.Seconds())))
		}

		// Log request to Redis Stream
		_, _ = rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: requestLogStream,
			Values: map[string]interface{}{
				"api_key":   apiKey,
				"endpoint":  endpoint,
				"count":     count,
				"limit":     limit,
				"status":    status,
				"timestamp": time.Now().Unix(),
			},
		}).Result()

		if count > limit {
			rateLimitedRequests.Inc()
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

// -------------------------------------------------------------------
// BASIC API HANDLERS
// -------------------------------------------------------------------

func testHandler(w http.ResponseWriter, r *http.Request) {

	hostname, _ := os.Hostname()
	fmt.Fprintf(w, "Test endpoint accessed | handled by: %s", hostname)
}

func ordersHandler(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	fmt.Fprintf(w, "Orders endpoint accessed | handled by: %s", hostname)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	fmt.Fprintf(w, "Login endpoint accessed | handled by: %s", hostname)
}

// Health check endpoint (used in production systems)
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "OK")
}

// -------------------------------------------------------------------
// ANALYTICS HANDLERS
// -------------------------------------------------------------------

func topUsersHandler(w http.ResponseWriter, r *http.Request) {

	query := `
	SELECT api_key, COUNT(*) as total_requests
	FROM request_logs
	GROUP BY api_key
	ORDER BY total_requests DESC
	LIMIT 10
	`

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, "Database error", 500)
		return
	}
	defer rows.Close()

	type Result struct {
		APIKey        string `json:"api_key"`
		TotalRequests int    `json:"total_requests"`
	}

	var results []Result

	for rows.Next() {

		var r Result

		err := rows.Scan(&r.APIKey, &r.TotalRequests)
		if err != nil {
			continue
		}

		results = append(results, r)
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(results)
}

func topEndpointsHandler(w http.ResponseWriter, r *http.Request) {

	query := `
	SELECT endpoint, COUNT(*) as total_requests
	FROM request_logs
	GROUP BY endpoint
	ORDER BY total_requests DESC
	LIMIT 10
	`

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, "Database error", 500)
		return
	}

	defer rows.Close()

	type Result struct {
		Endpoint      string `json:"endpoint"`
		TotalRequests int    `json:"total_requests"`
	}

	var results []Result

	for rows.Next() {

		var r Result

		err := rows.Scan(&r.Endpoint, &r.TotalRequests)
		if err != nil {
			continue
		}

		results = append(results, r)
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(results)
}

func blockedRequestsHandler(w http.ResponseWriter, r *http.Request) {

	query := `
	SELECT api_key, COUNT(*) as blocked_requests
	FROM request_logs
	WHERE status = 'blocked'
	GROUP BY api_key
	ORDER BY blocked_requests DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, "Database error", 500)
		return
	}

	defer rows.Close()

	type Result struct {
		APIKey          string `json:"api_key"`
		BlockedRequests int    `json:"blocked_requests"`
	}

	var results []Result

	for rows.Next() {

		var r Result

		err := rows.Scan(&r.APIKey, &r.BlockedRequests)
		if err != nil {
			continue
		}

		results = append(results, r)
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(results)
}

// Initialize MySQL connection
func initDB() *sql.DB {

	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbHost := os.Getenv("DB_HOST")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", dbUser, dbPass, dbHost, dbName)

	/*
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			panic(err)
		}

		err = db.Ping()
		if err != nil {
			panic("Database connection failed")
		}

		fmt.Println("MySQL connected")

		return db
	*/
	var db *sql.DB
	var err error

	// Retry loop (important)
	for i := 1; i <= 10; i++ {

		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
			if err == nil {
				fmt.Println("MySQL connected")
				return db
			}
		}

		fmt.Println("Waiting for MySQL... attempt", i)
		time.Sleep(3 * time.Second)
	}

	panic("Database connection failed after retries")
}

// Initialize Redis connection
func initRedis() *redis.Client {

	client := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	_, err := client.Ping(ctx).Result()
	if err != nil {
		panic("Redis connection failed")
	}

	fmt.Println("Redis connected")

	return client
}

// -------------------------------------------------------------------
// MAIN FUNCTION
// -------------------------------------------------------------------

func main() {

	var err error

	prometheus.MustRegister(totalRequests)
	prometheus.MustRegister(rateLimitedRequests)

	db = initDB()
	rdb = initRedis()

	fmt.Println("Gateway started on port 8080")

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/test", enableCORS(rateLimitMiddleware(testHandler)))
	http.HandleFunc("/orders", enableCORS(rateLimitMiddleware(ordersHandler)))
	http.HandleFunc("/login", enableCORS(rateLimitMiddleware(loginHandler)))

	http.HandleFunc("/analytics/top-users", enableCORS(topUsersHandler))
	http.HandleFunc("/analytics/top-endpoints", enableCORS(topEndpointsHandler))
	http.HandleFunc("/analytics/blocked-requests", enableCORS(blockedRequestsHandler))

	fs := http.FileServer(http.Dir("./dashboard"))
	http.Handle("/dashboard/", http.StripPrefix("/dashboard/", fs))

	http.HandleFunc("/health", enableCORS(healthHandler))

	err = http.ListenAndServe(":8080", nil)

	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
