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

// -------------------------------------------------------------------
// MIDDLEWARE: RATE LIMITING LOGIC USING REDIS
// This runs BEFORE actual API handler
// -------------------------------------------------------------------
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {

	// Return a new handler function
	return func(w http.ResponseWriter, r *http.Request) {

		// Step 1: Read API key from request header
		apiKey := r.Header.Get("x-api-key")

		if apiKey == "" {
			http.Error(w, "API key missing", http.StatusBadRequest)
			return
		}

		// Step 2: Create Redis key
		key := "rate_limit:" + apiKey

		// Step 3: Increment request count in Redis
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			http.Error(w, "Redis error", http.StatusInternalServerError)
			return
		}

		// Step 4: Set TTL for 60 seconds on first request
		if count == 1 {
			rdb.Expire(ctx, key, 60*time.Second)
		}

		// Step 5: Block if more than 5 requests
		if count > 5 {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Step 6: Allow request to reach actual handler
		next(w, r)
	}
}

// -------------------------------------------------------------------
// OLD HANDLER (kept for reference)
// func testHandler(w http.ResponseWriter, r *http.Request) {
// 	fmt.Fprintf(w, "API is working")
// }
// -------------------------------------------------------------------

// -------------------------------------------------------------------
// ACTUAL API HANDLER (NO RATE LIMIT LOGIC HERE)
// Only business logic should be here
// -------------------------------------------------------------------
func testHandler(w http.ResponseWriter, r *http.Request) {

	// Just confirm request passed middleware
	fmt.Fprintf(w, "Request allowed by rate limiter")
}

func main() {

	// Attach middleware BEFORE handler
	http.HandleFunc("/test", rateLimitMiddleware(testHandler))

	// Old way (no error handling)
	// http.ListenAndServe(":8080", nil)

	// Start server
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
