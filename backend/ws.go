package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// JobEvent is the payload broadcast to every subscriber of a given jobId.
type JobEvent struct {
	Type   string                 `json:"type"`
	JobID  string                 `json:"jobId"`
	Status string                 `json:"status"`
	Result map[string]interface{} `json:"result,omitempty"`
}

// wsClient wraps a connection with a per-conn write mutex. gorilla/websocket
// does not allow concurrent writers on the same connection.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteJSON(v)
}

// Hub tracks which WebSocket clients are subscribed to which jobId.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*wsClient]struct{}
}

func newHub() *Hub {
	return &Hub{subscribers: map[string]map[*wsClient]struct{}{}}
}

func (h *Hub) subscribe(jobID string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[jobID]; !ok {
		h.subscribers[jobID] = map[*wsClient]struct{}{}
	}
	h.subscribers[jobID][c] = struct{}{}
}

func (h *Hub) unsubscribe(jobID string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subscribers[jobID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.subscribers, jobID)
		}
	}
}

// Broadcast sends an event to every client currently subscribed to jobID.
// Clients that fail to receive are dropped.
func (h *Hub) Broadcast(event JobEvent) {
	h.mu.RLock()
	clients := make([]*wsClient, 0, len(h.subscribers[event.JobID]))
	for c := range h.subscribers[event.JobID] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		if err := c.writeJSON(event); err != nil {
			fmt.Printf("[WS] dropping client for %s: %v\n", event.JobID, err)
			_ = c.conn.Close()
			h.unsubscribe(event.JobID, c)
		}
	}
}

var hub = newHub()

var wsUpgrader = websocket.Upgrader{
	// Permissive origin check — fine for a local demo. Tighten for prod.
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "missing jobId", http.StatusBadRequest)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("[WS] upgrade failed: %v\n", err)
		return
	}

	client := &wsClient{conn: conn}
	hub.subscribe(jobID, client)
	fmt.Printf("[WS] client subscribed to %s\n", jobID)

	// Send the current state immediately so clients that connect after the
	// terminal event don't get stuck waiting forever.
	if job, err := getJob(jobID); err == nil {
		_ = client.writeJSON(JobEvent{
			Type:   "snapshot",
			JobID:  job.ID,
			Status: job.Status,
			Result: job.Result,
		})
	}

	// Keepalive: ping every 30s; enforce a read deadline so dead peers get
	// cleaned up. We don't expect messages from the client, but we must drain
	// the read side for control frames (ping/pong/close) to be processed.
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			hub.unsubscribe(jobID, client)
			_ = conn.Close()
			fmt.Printf("[WS] client disconnected from %s\n", jobID)
			return
		case <-ticker.C:
			client.mu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			client.mu.Unlock()
			if err != nil {
				hub.unsubscribe(jobID, client)
				_ = conn.Close()
				return
			}
		}
	}
}

// broadcastJob is a small helper so other files don't need to import gorilla.
func broadcastJob(job Job, eventType string) {
	hub.Broadcast(JobEvent{
		Type:   eventType,
		JobID:  job.ID,
		Status: job.Status,
		Result: job.Result,
	})
}
