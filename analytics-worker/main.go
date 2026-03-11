package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

var rdb = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

var streamName = "request_logs_stream"

func main() {

	fmt.Println("Worker started. Listening to Redis stream...")

	// MySQL configuration
	cfg := mysql.Config{
		User:                 "root",
		Passwd:               "rootpassword",
		Net:                  "tcp",
		Addr:                 "127.0.0.1:3306",
		DBName:               "api_gateway_analytics",
		AllowNativePasswords: true,
	}

	// Connect to MySQL
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		panic(err)
	}

	defer db.Close()

	lastID := "0"

	for {

		streams, err := rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamName, lastID},
			Block:   0,
		}).Result()

		if err != nil {
			fmt.Println("Stream read error:", err)
			continue
		}

		for _, stream := range streams {
			for _, message := range stream.Messages {

				values := message.Values

				apiKey := values["api_key"].(string)
				endpoint := values["endpoint"].(string)
				status := values["status"].(string)

				count := values["count"]
				limit := values["limit"]
				timestamp := values["timestamp"]

				// Insert into MySQL
				query := `
				INSERT INTO request_logs
				(api_key, endpoint, request_count, request_limit, status, timestamp)
				VALUES (?, ?, ?, ?, ?, ?)`

				_, err := db.Exec(query, apiKey, endpoint, count, limit, status, timestamp)

				if err != nil {
					fmt.Println("DB insert error:", err)
				} else {
					fmt.Println("Inserted log into MySQL:", apiKey, endpoint)
				}

				lastID = message.ID
			}
		}
	}
}
