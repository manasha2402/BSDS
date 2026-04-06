package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// replicatePayload is what the Write Coordinator sends to every peer
// via POST /internal/set
type replicatePayload struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// propagateToAll sends the key/value/version to every peer concurrently
// and waits for ALL of them to acknowledge (W=N).
//
// This is simpler than the leader-follower replication because there
// is no configurable quorum — we always need every node to ack.
// If any peer fails, the whole write is considered failed.
func propagateToAll(key, value string, version int64) error {
	if len(cfg.Peers) == 0 {
		return nil // single node cluster, nothing to propagate
	}

	type result struct {
		peer string
		err  error
	}

	results := make(chan result, len(cfg.Peers))

	// Fan out to all peers concurrently.
	for _, peer := range cfg.Peers {
		go func(p string) {
			err := sendToPeer(p, key, value, version)
			results <- result{peer: p, err: err}
		}(peer)
	}

	// Collect all results — W=N means we wait for every single peer.
	var mu sync.Mutex
	var failures []string

	for i := 0; i < len(cfg.Peers); i++ {
		res := <-results
		if res.err != nil {
			mu.Lock()
			failures = append(failures, fmt.Sprintf("%s: %v", res.peer, res.err))
			mu.Unlock()
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("propagation failed for peers: %v", failures)
	}
	return nil
}

// sendToPeer sends a single replication message to one peer node.
// The Write Coordinator sleeps 200ms before each send
// (assignment requirement — simulates real network/storage latency).
func sendToPeer(peer, key, value string, version int64) error {
	// Simulated coordinator-side delay per the assignment spec.
	time.Sleep(200 * time.Millisecond)

	payload := replicatePayload{Key: key, Value: value, Version: version}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(peer+"/internal/set", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("peer %s unreachable: %w", peer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer %s returned status %d", peer, resp.StatusCode)
	}
	return nil
}
