package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

// -------------------------------------------------------------------
// REDIS SETUP
// -------------------------------------------------------------------

var ctx = context.Background()

var rdb = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

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

var userPlans = map[string]int64{
	"user123": 5,
	"proUser": 20,
}

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
// RATE LIMITING MIDDLEWARE
// -------------------------------------------------------------------

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		apiKey := r.Header.Get("x-api-key")

		if apiKey == "" {
			http.Error(w, "API key missing", http.StatusBadRequest)
			return
		}

		endpoint := r.URL.Path

		limit, exists := userPlans[apiKey]
		if !exists {
			limit = requestLimit
		}

		if endpointLimit, ok := endpointLimits[endpoint]; ok {
			limit = endpointLimit
		}

		key := "rate_limit:" + apiKey + ":" + endpoint

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			http.Error(w, "Redis error", http.StatusInternalServerError)
			return
		}

		if count == 1 {
			rdb.Expire(ctx, key, 60*time.Second)
		}

		remaining := limit - count

		status := "allowed"
		if count > limit {
			status = "blocked"
		}

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
	fmt.Fprintf(w, "Request allowed by rate limiter")
}

func ordersHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Orders endpoint accessed")
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Login endpoint accessed")
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

		rows.Scan(&r.APIKey, &r.TotalRequests)

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

		rows.Scan(&r.Endpoint, &r.TotalRequests)

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

		rows.Scan(&r.APIKey, &r.BlockedRequests)

		results = append(results, r)
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(results)
}

// -------------------------------------------------------------------
// MAIN FUNCTION
// -------------------------------------------------------------------

func main() {

	var err error

	db, err = sql.Open("mysql", "root:rootpassword@tcp(127.0.0.1:3306)/api_gateway_analytics")

	if err != nil {
		panic(err)
	}

	fmt.Println("Gateway started on port 8080")

	http.HandleFunc("/test", enableCORS(rateLimitMiddleware(testHandler)))
	http.HandleFunc("/orders", enableCORS(rateLimitMiddleware(ordersHandler)))
	http.HandleFunc("/login", enableCORS(rateLimitMiddleware(loginHandler)))

	http.HandleFunc("/analytics/top-users", enableCORS(topUsersHandler))
	http.HandleFunc("/analytics/top-endpoints", enableCORS(topEndpointsHandler))
	http.HandleFunc("/analytics/blocked-requests", enableCORS(blockedRequestsHandler))

	err = http.ListenAndServe(":8080", nil)

	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
