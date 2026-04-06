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
// Unlike leader-follower, ANY node can receive this — the receiving
// node becomes the Write Coordinator for this write.
func handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{"POST only"})
		return
	}

	var req SetRequest
	if err := decodeJSON(r, &req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{"invalid request or empty key"})
		return
	}

	// This node is now the Write Coordinator.
	// Generate a version number based on what this node knows locally.
	version := store.NextVersion(req.Key)

	// Write locally first so the coordinator is always consistent
	// after the write completes.
	store.Set(req.Key, req.Value, version)

	// Propagate to all peers and wait for all to ack (W=N).
	// This blocks until every peer has written the value.
	if err := propagateToAll(req.Key, req.Value, version); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"key":     req.Key,
		"version": version,
	})
}

// handleGet serves read requests from clients.
// R=1: just return the local value on this node — fast but
// potentially stale if this node hasn't received the latest write yet.
// This is intentional — it exposes the inconsistency window.
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

	// Simulated read delay — same as follower delay in leader-follower.
	time.Sleep(50 * time.Millisecond)

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

// ---------- internal / test endpoints ----------

// handleHealth is a simple health check endpoint for the ALB.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLocalRead returns the raw local value on this node.
// No delay, no coordination — used by unit tests to catch
// stale data within the inconsistency window.
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

// handleInternalSet is called by the Write Coordinator on peer nodes.
// It writes the value directly to the local store.
// Sleeps 100ms to simulate storage write latency (assignment requirement).
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
