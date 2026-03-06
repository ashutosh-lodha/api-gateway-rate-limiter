package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis context
// Required for all Redis operations (timeouts, lifecycle, cancellation support)
var ctx = context.Background()

// Redis client
// Connects to Redis running on localhost:6379 (Docker container)
var rdb = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

// Rate limit configuration
// Maximum requests allowed per time window
var requestLimit int64 = 5

// Endpoint-specific rate limits - Some APIs require stricter control (e.g. login)
var endpointLimits = map[string]int64{
	"/login":  3,
	"/orders": 10,
}

// Simple user plan configuration - In real systems this would come from database
var userPlans = map[string]int64{
	"user123": 5,
	"proUser": 20,
}

// -------------------------------------------------------------------
// MIDDLEWARE: RATE LIMITING LOGIC USING REDIS - This runs BEFORE actual API handler
// -------------------------------------------------------------------
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		// Step 1: Read API key from request header
		apiKey := r.Header.Get("x-api-key")

		if apiKey == "" {
			http.Error(w, "API key missing", http.StatusBadRequest)
			return
		}

		// Identify which endpoint the user is accessing
		endpoint := r.URL.Path

		// determine request limit for this user
		limit, exists := userPlans[apiKey]
		if !exists {
			limit = requestLimit
		}

		// Check if endpoint has its own limit
		endpointLimit, endpointExists := endpointLimits[endpoint]
		if endpointExists {
			limit = endpointLimit
		}

		// Create Redis key (user + endpoint)
		key := "rate_limit:" + apiKey + ":" + endpoint

		// Increment request count
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			http.Error(w, "Redis error", http.StatusInternalServerError)
			return
		}

		// Set expiry on first request
		if count == 1 {
			rdb.Expire(ctx, key, 60*time.Second)
		}

		// Remaining requests
		remaining := limit - count

		// Send rate limit headers
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		// Get reset time
		ttl, err := rdb.TTL(ctx, key).Result()
		if err == nil {
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", int(ttl.Seconds())))
		}

		// Block if limit exceeded
		if count > limit {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Allow request
		next(w, r)
	}
}

// -------------------------------------------------------------------
// OLD HANDLER (kept for reference)
// func testHandler(w http.ResponseWriter, r *http.Request) {
// 	fmt.Fprintf(w, "API is working")
// }
// -------------------------------------------------------------------

// ACTUAL API HANDLER (NO RATE LIMIT LOGIC HERE)
// Only business logic should be here
// -------------------------------------------------------------------
func testHandler(w http.ResponseWriter, r *http.Request) {

	// Just confirm request passed middleware
	fmt.Fprintf(w, "Request allowed by rate limiter")
}

// Second API endpoint used to demonstrate middleware protecting multiple routes
func ordersHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Orders endpoint accessed")
}

// Login endpoint - Used to demonstrate stricter rate limiting for sensitive APIs
func loginHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Login endpoint accessed")
}

func main() {

	// Attach middleware BEFORE handler
	http.HandleFunc("/test", rateLimitMiddleware(testHandler))
	http.HandleFunc("/orders", rateLimitMiddleware(ordersHandler))
	http.HandleFunc("/login", rateLimitMiddleware(loginHandler))

	// Old way (no error handling)
	// http.ListenAndServe(":8080", nil)

	// Start server
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
