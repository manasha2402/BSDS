package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// ---------- request / response shapes ----------

type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type GetResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ---------- public API ----------

// handleSet is the entry point for all client write requests.
// Only the leader should receive these in practice.
func handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{"POST only"})
		return
	}

	var req SetRequest
	if err := decodeJSON(r, &req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"invalid request body or empty key"})
		return
	}

	// Only the leader handles writes; followers reject them.
	if cfg.Role != "leader" {
		writeJSON(w, http.StatusForbidden, ErrorResponse{"not the leader"})
		return
	}

	// Generate the next version number for this key.
	version := store.NextVersion(req.Key)

	// Replicate to followers according to the W quorum.
	// replicate() writes to local store and peers and blocks
	// until W acknowledgements are collected.
	if err := replicate(req.Key, req.Value, version); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key":     req.Key,
		"version": version,
	})
}

// handleGet serves read requests from clients.
// Both leader and followers can serve reads.
func handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{"GET only"})
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"missing key"})
		return
	}

	// quorumRead contacts R nodes (including self) and returns
	// the entry with the highest version number.
	entry, err := quorumRead(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   entry.Value,
		Version: entry.Version,
	})
}

// ---------- internal / test endpoints ----------

// handleHealth is a simple health check endpoint for the ALB.
// Returns 200 OK so the load balancer knows the container is alive.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLocalRead returns the value stored locally on this node only.
// It bypasses quorum entirely — used by the leader during quorum reads
// (via fetchFromPeer) and by unit tests to catch stale data mid-window.
//
// The assignment spec requires followers to sleep 50ms when serving a
// read request from the leader. That call path goes through this endpoint,
// so the delay lives here rather than in handleGet.
func handleLocalRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{"GET only"})
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"missing key"})
		return
	}

	// Simulated read delay — applied when a follower responds to the
	// leader's quorum-read fetch (assignment requirement).
	if cfg.Role == "follower" {
		time.Sleep(50 * time.Millisecond)
	}

	entry, ok := store.Get(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{"key not found"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   entry.Value,
		Version: entry.Version,
	})
}

// handleInternalSet is called by the leader on each follower during replication.
// It writes the value directly to the local store (no further replication).
// The follower sleeps 100ms to simulate storage write latency.
func handleInternalSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{"POST only"})
		return
	}

	var req struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Version int64  `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"invalid body"})
		return
	}

	// Simulated storage write delay as required by the assignment.
	time.Sleep(100 * time.Millisecond)

	store.Set(req.Key, req.Value, req.Version)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
