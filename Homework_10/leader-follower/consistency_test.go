package main

// Consistency tests for the Leader-Follower KV cluster.
//
// The cluster must be running before these tests are executed:
//
//	W=5 R=1 docker compose up --build   (or any W/R combo)
//	go test -v -run TestLeaderFollower ./leader-follower/
//
// Node URLs default to the docker-compose ports but can be overridden
// via environment variables:
//
//	LF_LEADER=http://localhost:8080
//	LF_FOLLOWERS=http://localhost:8081,http://localhost:8082,...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── cluster URL helpers ───────────────────────────────────────────────────────

func leaderURL() string {
	if u := os.Getenv("LF_LEADER"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8080"
}

func followerURLs() []string {
	if env := os.Getenv("LF_FOLLOWERS"); env != "" {
		var urls []string
		for _, u := range strings.Split(env, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				urls = append(urls, strings.TrimRight(u, "/"))
			}
		}
		return urls
	}
	return []string{
		"http://localhost:8081",
		"http://localhost:8082",
		"http://localhost:8083",
		"http://localhost:8084",
	}
}

// ── thin HTTP wrappers ────────────────────────────────────────────────────────

type kvResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	Error   string `json:"error"`
}

// kvSet sends POST /set to the given node and returns the version number.
// Fails the test immediately on a non-201 response.
func kvSet(t *testing.T, nodeURL, key, value string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"key": key, "value": value})
	resp, err := http.Post(nodeURL+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /set to %s failed: %v", nodeURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /set to %s: expected 201, got %d", nodeURL, resp.StatusCode)
	}
	var r kvResponse
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Version
}

// kvGet sends GET /get?key= to the given node.
// Returns (value, version, found).
func kvGet(nodeURL, key string) (string, int64, bool) {
	resp, err := http.Get(fmt.Sprintf("%s/get?key=%s", nodeURL, key))
	if err != nil || resp.StatusCode == http.StatusNotFound {
		if resp != nil {
			resp.Body.Close()
		}
		return "", 0, false
	}
	defer resp.Body.Close()
	var r kvResponse
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Value, r.Version, true
}

// kvLocalRead sends GET /local_read?key= to the given node.
// Returns (value, version, found). Does not trigger any quorum logic.
func kvLocalRead(nodeURL, key string) (string, int64, bool) {
	resp, err := http.Get(fmt.Sprintf("%s/local_read?key=%s", nodeURL, key))
	if err != nil || resp.StatusCode == http.StatusNotFound {
		if resp != nil {
			resp.Body.Close()
		}
		return "", 0, false
	}
	defer resp.Body.Close()
	var r kvResponse
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Value, r.Version, true
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestSetThenReadLeader: after the leader acks a write, reading from the leader
// must return the same value (never stale, regardless of W).
func TestSetThenReadLeader(t *testing.T) {
	leader := leaderURL()
	key := fmt.Sprintf("test-leader-%d", time.Now().UnixNano())
	value := "hello-leader"

	writtenVersion := kvSet(t, leader, key, value)

	readValue, readVersion, found := kvGet(leader, key)
	if !found {
		t.Fatalf("key %q not found on leader after write", key)
	}
	if readValue != value {
		t.Errorf("leader read mismatch: got %q, want %q", readValue, value)
	}
	if readVersion != writtenVersion {
		t.Errorf("leader version mismatch: got %d, want %d", readVersion, writtenVersion)
	}
	t.Logf("leader read consistent: key=%s value=%s version=%d", key, readValue, readVersion)
}

// TestSetThenReadFollower: after the leader acks a write, reading from any
// follower must return consistent data.
// For W=5 this is guaranteed; for lower W the quorum read (R) handles it.
func TestSetThenReadFollower(t *testing.T) {
	leader := leaderURL()
	followers := followerURLs()
	key := fmt.Sprintf("test-follower-%d", time.Now().UnixNano())
	value := "hello-follower"

	writtenVersion := kvSet(t, leader, key, value)

	for _, f := range followers {
		readValue, readVersion, found := kvGet(f, key)
		if !found {
			t.Errorf("follower %s: key %q not found after leader ack", f, key)
			continue
		}
		if readValue != value {
			t.Errorf("follower %s stale value: got %q, want %q", f, readValue, value)
		}
		if readVersion != writtenVersion {
			t.Errorf("follower %s stale version: got %d, want %d", f, readVersion, writtenVersion)
		}
		t.Logf("follower %s consistent: value=%s version=%d", f, readValue, readVersion)
	}
}

// TestLocalReadInconsistencyWindow: sends a write to the leader, then
// immediately hits /local_read on followers before replication can complete.
// This intentionally tries to catch stale state — it will often succeed
// (show staleness) under load or with slow followers.
//
// The test does NOT fail on a stale read — staleness here is the *expected*
// observable behaviour during the propagation window. It logs the result.
func TestLocalReadInconsistencyWindow(t *testing.T) {
	leader := leaderURL()
	followers := followerURLs()

	const iterations = 10
	var staleCount int64

	for i := 0; i < iterations; i++ {
		key := fmt.Sprintf("test-window-%d-%d", time.Now().UnixNano(), i)
		value := fmt.Sprintf("val-%d", i)

		// Start polling followers via local_read BEFORE issuing the write
		// so that some polls land inside the propagation window.
		stopPolling := make(chan struct{})
		var wg sync.WaitGroup

		for _, f := range followers {
			wg.Add(1)
			go func(follower string) {
				defer wg.Done()
				for {
					select {
					case <-stopPolling:
						return
					default:
						v, ver, found := kvLocalRead(follower, key)
						if found && ver > 0 {
							_ = v
						}
						_ = ver
						time.Sleep(5 * time.Millisecond)
					}
				}
			}(f)
		}

		// Issue the write to the leader.
		writtenVersion := kvSet(t, leader, key, value)

		// Read local_read on each follower immediately — race against replication.
		for _, f := range followers {
			v, ver, found := kvLocalRead(f, key)
			if found && ver < writtenVersion {
				atomic.AddInt64(&staleCount, 1)
				t.Logf("[STALE] follower=%s key=%s local_version=%d written_version=%d value=%q",
					f, key, ver, writtenVersion, v)
			}
		}

		close(stopPolling)
		wg.Wait()
	}

	t.Logf("Inconsistency window test: %d stale local_reads out of %d opportunities (%d followers × %d iterations)",
		staleCount, int64(len(followers)*iterations), len(followers), iterations)
	t.Log("Note: 0 stale reads is correct for W=5. For W=1 or W=3 you should see some stale reads here.")
}

// TestMultipleWritesVersionMonotonicity: verifies that version numbers
// increase monotonically with each write to the same key, on both leader
// and followers (after ack).
func TestMultipleWritesVersionMonotonicity(t *testing.T) {
	leader := leaderURL()
	followers := followerURLs()
	key := fmt.Sprintf("test-mono-%d", time.Now().UnixNano())

	var lastVersion int64
	for i := 0; i < 5; i++ {
		value := fmt.Sprintf("v%d", i)
		version := kvSet(t, leader, key, value)

		if version <= lastVersion {
			t.Errorf("write %d: version %d not greater than previous %d", i, version, lastVersion)
		}
		lastVersion = version

		// Confirm each follower sees the updated version via quorum read.
		for _, f := range followers {
			_, readVer, found := kvGet(f, key)
			if !found {
				t.Errorf("follower %s: key not found after write %d", f, i)
				continue
			}
			if readVer != version {
				t.Errorf("follower %s: version %d, want %d after write %d", f, readVer, version, i)
			}
		}
	}
	t.Logf("All 5 writes had monotonically increasing versions, consistent on all followers")
}
