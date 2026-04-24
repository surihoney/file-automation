package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

var rdb = redis.NewClient(&redis.Options{
	Addr: "redis:6379",
})

func init() {
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		panic("Redis connection failed: " + err.Error())
	}
}

func saveJob(job Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	key := "job:" + job.ID

	fmt.Printf("[REDIS] Writing key: %s\n", key)

	err = rdb.Set(ctx, key, data, 0).Err()
	if err != nil {
		fmt.Printf("[REDIS] WRITE FAILED: %v\n", err)
		return err
	}

	fmt.Println("[REDIS] WRITE SUCCESS")
	return nil
}
