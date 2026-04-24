package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const uploadDir = "./uploads"

func main() {

	startWorker()

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		fmt.Printf("Failed to create upload dir: %v\n", err)
	}

	http.HandleFunc("/upload", withCORS(uploadHandler))
	http.HandleFunc("/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	http.HandleFunc("/job/update", updateJobHandler)
	http.HandleFunc("/job/status", withCORS(statusHandler))
	http.HandleFunc("/ws", wsHandler)

	fmt.Println("Backend running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// withCORS adds permissive CORS headers so the frontend can talk to the
// backend directly during demos even without the Vite proxy.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func enqueueJob(job Job) error {
	data, _ := json.Marshal(job)

	return rdb.LPush(ctx, "jobs", data).Err()
}

// uploadHandler accepts a multipart/form-data POST with a "file" field,
// persists it to ./uploads, and forwards the filename to n8n.
func uploadHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// 32MB is plenty for a demo.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form: " + err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field: " + err.Error()})
		return
	}
	defer file.Close()

	safeName := filepath.Base(header.Filename)
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	stored := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safeName)
	destPath := filepath.Join(uploadDir, stored)

	job := Job{
		ID:       jobID,
		Filename: safeName,
		StoredAs: stored,
		Status:   "PENDING",
	}

	if err := saveJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save job: " + err.Error()})
		return
	}

	if err := enqueueJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job: " + err.Error()})
		return
	}

	dst, err := os.Create(destPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create file: " + err.Error()})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save file: " + err.Error()})
		return
	}

	fmt.Printf("Saved %s (%d bytes) -> %s\n", safeName, written, destPath)

	resp := map[string]any{
		"jobId":     jobID,
		"filename":  safeName,
		"stored_as": stored,
		"size":      written,
		"status":    "queued",
	}
	writeJSON(w, http.StatusOK, resp)
}

func triggerN8N(jobID, filename, storedAs string) error {
	payload := map[string]string{
		"jobId":    jobID,
		"filename": filename,
		"storedAs": storedAs,
	}

	jsonData, _ := json.Marshal(payload)

	resp, err := http.Post(
		"http://n8n:5678/webhook/file-process",
		"application/json",
		bytes.NewBuffer(jsonData),
	)

	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}

func updateJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ID     string                 `json:"jobId"`
		Status string                 `json:"status"`
		Result map[string]interface{} `json:"result"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", 400)
		return
	}

	fmt.Printf("[REDIS] Looking for job ID: %s\n", payload.ID)

	if payload.ID == "" {
		http.Error(w, "missing jobId", 400)
		return
	}
	job, err := getJob(payload.ID)
	if err != nil {
		http.Error(w, "job not found", 404)
		return
	}

	job.Status = payload.Status
	job.Result = payload.Result

	err = saveJob(*job)
	if err != nil {
		http.Error(w, "failed to update job", 500)
		return
	}

	fmt.Printf("[REDIS] Job updated: %+v\n", job)

	broadcastJob(*job, "update")

	w.Write([]byte("ok"))
}

func getJob(id string) (*Job, error) {
	val, err := rdb.Get(ctx, "job:"+id).Result()
	if err != nil {
		return nil, err
	}

	var job Job
	json.Unmarshal([]byte(val), &job)
	return &job, nil
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id"})
		return
	}

	job, err := getJob(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	writeJSON(w, http.StatusOK, job)
}
