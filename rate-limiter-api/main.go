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

// Old test handler (kept for reference - basic health check)
// func testHandler(w http.ResponseWriter, r *http.Request) {
// 	fmt.Fprintf(w, "API is working")
// }

func testHandler(w http.ResponseWriter, r *http.Request) {

	// Read API key from incoming request header
	apiKey := r.Header.Get("x-api-key")

	// If API key not provided, block request
	if apiKey == "" {
		http.Error(w, "API key missing", http.StatusBadRequest)
		return
	}
	// OLD LOGIC (only printing API key - before rate limiting was added)
	// fmt.Fprintf(w, "Your API key is: %s", apiKey)
	// -------------------------------------------------------------------
	// NEW LOGIC: RATE LIMITING USING REDIS
	// -------------------------------------------------------------------
	// Create a Redis key for this user
	// Example:
	// API key = user123
	// Redis key becomes = rate_limit:user123
	key := "rate_limit:" + apiKey

	// Increment request count in Redis
	// INCR operation:
	// If key doesn't exist → creates key with value 1
	// If key exists → increments value by 1
	count, err := rdb.Incr(ctx, key).Result()

	// If Redis operation fails
	if err != nil {
		http.Error(w, "Redis error", http.StatusInternalServerError)
		return
	}

	// If this is the first request from this user
	// set TTL = 60 seconds
	if count == 1 {

		// OLD WAY (manual nanoseconds - hard to read)
		// rdb.Expire(ctx, key, 60*1000000000)
		// BETTER WAY using time package
		rdb.Expire(ctx, key, 60*time.Second)
	}

	// If more than 5 requests in 60 seconds → block
	if count > 5 {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Allow request if under limit
	fmt.Fprintf(w, "Allowed. Request count: %d", count)
}

func main() {

	// Map /test endpoint to testHandler function
	http.HandleFunc("/test", testHandler)

	// Old way (no error handling)
	// http.ListenAndServe(":8080", nil)

	// Start server with error handling
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
