package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// replicatePayload is what the leader sends to each follower
// via POST /internal/set
type replicatePayload struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// replicate writes the key/value/version to the local store first,
// then fans out to all followers concurrently.
//
// The W quorum controls how many nodes (including the leader itself)
// must acknowledge the write before we return 201 to the client.
//
//	W=1 → leader writes locally and returns immediately (lazy replication)
//	W=3 → leader + 2 followers must ack before returning
//	W=5 → all 5 nodes must ack before returning
func replicate(key, value string, version int64) error {
	// Always write to the leader's local store first.
	store.Set(key, value, version)

	// W=1 means only the leader needs to have written — return now.
	// Followers will be updated asynchronously.
	if cfg.W == 1 {
		go spreadToFollowers(key, value, version) // fire and forget
		return nil
	}

	// For W > 1 we need acks from (W - 1) followers
	// (the leader itself counts as 1).
	needed := cfg.W - 1
	if needed <= 0 {
		return nil
	}

	type result struct {
		err error
	}
	results := make(chan result, len(cfg.Peers))

	for _, peer := range cfg.Peers {
		go func(peer string) {
			results <- result{err: sendToPeer(peer, key, value, version)}
		}(peer)
	}

	// Collect exactly `needed` successes. If we get enough, return
	// success even if some followers are still in flight.
	acks, errs := 0, 0
	for i := 0; i < len(cfg.Peers); i++ {
		res := <-results
		if res.err == nil {
			acks++
			if acks >= needed {
				return nil // quorum reached
			}
		} else {
			errs++
			// If remaining peers can't possibly reach quorum, fail fast.
			remaining := len(cfg.Peers) - i - 1
			if acks+remaining < needed {
				return fmt.Errorf("could not reach write quorum W=%d, only %d acks", cfg.W, acks+1)
			}
		}
	}

	if acks >= needed {
		return nil
	}
	return fmt.Errorf("write quorum not met: got %d acks (needed %d)", acks+1, cfg.W)
}

// spreadToFollowers sends to all followers without waiting for acks.
// Used only for W=1 (fire-and-forget background replication).
// sendToPeer already applies the required 200ms delay, so no extra sleep here.
func spreadToFollowers(key, value string, version int64) {
	for _, peer := range cfg.Peers {
		go func(p string) {
			_ = sendToPeer(p, key, value, version)
		}(peer)
	}
}

// sendToPeer sends a single replication message to one follower.
// The leader sleeps 200ms before sending (assignment requirement).
func sendToPeer(peer, key, value string, version int64) error {
	log.Printf("[replication] sending key=%s version=%d to peer=%s", key, version, peer)
	// Simulated network + leader-side delay per the assignment spec.
	time.Sleep(200 * time.Millisecond)

	payload := replicatePayload{Key: key, Value: value, Version: version}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(peer+"/internal/set", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[replication] peer %s unreachable: %v", peer, err)
		return fmt.Errorf("peer %s unreachable: %w", peer, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[replication] peer %s returned %d", peer, resp.StatusCode)
		return fmt.Errorf("peer %s returned %d", peer, resp.StatusCode)
	}
	log.Printf("[replication] peer %s acked key=%s version=%d", peer, key, version)
	return nil
}

// ---------- Read quorum ----------

// quorumRead fetches from R nodes and returns the entry with
// the highest version number — this is how stale reads are avoided
// when R > 1.
//
//	R=1 → just read from local store (fast, possibly stale)
//	R=3 → read from self + 2 peers, return freshest
//	R=5 → read from all 5 nodes, return freshest
func quorumRead(key string) (Entry, error) {
	// R=1: just return local value, no network calls needed.
	if cfg.R == 1 {
		e, ok := store.Get(key)
		if !ok {
			return Entry{}, fmt.Errorf("key not found: %s", key)
		}
		return e, nil
	}

	type result struct {
		entry Entry
		found bool
		err   error
	}

	results := make(chan result, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		go func(peer string) {
			e, err := fetchFromPeer(peer, key)
			if err != nil {
				// key not found on peer counts as version 0
				results <- result{entry: Entry{Version: 0}, found: false, err: nil}
			} else {
				results <- result{entry: e, found: true, err: nil}
			}
		}(peer)
	}

	// Start with local value
	best, ok := store.Get(key)
	if !ok {
		best = Entry{Version: 0}
	}
	acks := 1 // count self

	for i := 0; i < len(cfg.Peers); i++ {
		res := <-results
		acks++
		if res.found && res.entry.Version > best.Version {
			best = res.entry
		}
		if acks >= cfg.R {
			if best.Version == 0 {
				return Entry{}, fmt.Errorf("key not found: %s", key)
			}
			return best, nil
		}
	}

	if best.Version == 0 {
		return Entry{}, fmt.Errorf("key not found: %s", key)
	}
	return best, nil
}

// fetchFromPeer does a GET /local_read on a follower node.
// We use local_read (not /get) so we get the raw local value
// without triggering another quorum read on the peer.
// The follower sleeps 50ms before responding (assignment requirement).
func fetchFromPeer(peer, key string) (Entry, error) {
	url := fmt.Sprintf("%s/local_read?key=%s", peer, key)
	resp, err := http.Get(url)
	if err != nil {
		return Entry{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Entry{}, fmt.Errorf("key not found on peer %s", peer)
	}
	if resp.StatusCode != http.StatusOK {
		return Entry{}, fmt.Errorf("peer %s returned %d", peer, resp.StatusCode)
	}

	var gr GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return Entry{}, err
	}
	return Entry{Value: gr.Value, Version: gr.Version}, nil
}
