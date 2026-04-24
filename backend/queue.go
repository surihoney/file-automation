package main

import (
	"encoding/json"
	"fmt"
)

type Job struct {
	ID       string                 `json:"id"`
	Filename string                 `json:"filename"`
	StoredAs string                 `json:"storedAs"`
	Status   string                 `json:"status"`
	Result   map[string]interface{} `json:"result,omitempty"`
}

func startWorker() {
	go func() {
		for {
			result, err := rdb.BRPop(ctx, 0, "jobs").Result()
			if err != nil {
				fmt.Println("worker error:", err)
				continue
			}

			var job Job
			json.Unmarshal([]byte(result[1]), &job)

			fmt.Printf("[WORKER] Processing %s\n", job.ID)

			if err := triggerN8N(job.ID, job.Filename, job.StoredAs); err != nil {
				fmt.Printf("[WORKER] Failed %s: %v\n", job.ID, err)
				job.Status = "FAILED"
				if saveErr := saveJob(job); saveErr != nil {
					fmt.Printf("[WORKER] Could not persist FAILED status for %s: %v\n", job.ID, saveErr)
				}
				broadcastJob(job, "update")
				continue
			}

			job.Status = "TRIGGERED"
			if err := saveJob(job); err != nil {
				fmt.Printf("[WORKER] Could not persist TRIGGERED status for %s: %v\n", job.ID, err)
			}
			broadcastJob(job, "update")

			fmt.Printf("[WORKER] Triggered %s\n", job.ID)
		}
	}()
}
