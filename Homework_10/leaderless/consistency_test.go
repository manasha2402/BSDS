package main

// Consistency tests for the Leaderless KV cluster.
//
// The cluster must be running before executing these tests:
//
//	docker compose up --build
//	go test -v -run TestLeaderless ./leaderless/
//
// Node URLs default to the docker-compose ports but can be overridden:
//
//	LL_NODES=http://localhost:8081,http://localhost:8082,...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ── cluster URL helpers ───────────────────────────────────────────────────────

func nodeURLs() []string {
	if env := os.Getenv("LL_NODES"); env != "" {
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
		"http://localhost:8085",
	}
}

func randomNode(nodes []string) string {
	return nodes[rand.Intn(len(nodes))]
}

func otherNodes(nodes []string, exclude string) []string {
	var rest []string
	for _, n := range nodes {
		if n != exclude {
			rest = append(rest, n)
		}
	}
	return rest
}

// ── thin HTTP wrappers ────────────────────────────────────────────────────────

type kvResp struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	Error   string `json:"error"`
}

func llSet(t *testing.T, nodeURL, key, value string) int64 {
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
	var r kvResp
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Version
}

func llGet(nodeURL, key string) (string, int64, bool) {
	resp, err := http.Get(fmt.Sprintf("%s/get?key=%s", nodeURL, key))
	if err != nil || resp.StatusCode == http.StatusNotFound {
		if resp != nil {
			resp.Body.Close()
		}
		return "", 0, false
	}
	defer resp.Body.Close()
	var r kvResp
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Value, r.Version, true
}

func llLocalRead(nodeURL, key string) (string, int64, bool) {
	resp, err := http.Get(fmt.Sprintf("%s/local_read?key=%s", nodeURL, key))
	if err != nil || resp.StatusCode == http.StatusNotFound {
		if resp != nil {
			resp.Body.Close()
		}
		return "", 0, false
	}
	defer resp.Body.Close()
	var r kvResp
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Value, r.Version, true
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestInconsistencyWindow: writes to a random node (Write Coordinator) and
// immediately reads from other nodes via /get before propagation completes.
//
// Because W=N the coordinator blocks until all peers ack, but we fire reads
// concurrently with the write so some land inside the propagation window.
// Stale reads here are the *expected* behaviour this test is designed to show.
func TestInconsistencyWindow(t *testing.T) {
	nodes := nodeURLs()
	const iterations = 20
	var staleCount int64
	var totalReads int64

	for i := 0; i < iterations; i++ {
		coordinator := randomNode(nodes)
		peers := otherNodes(nodes, coordinator)
		key := fmt.Sprintf("ll-window-%d-%d", time.Now().UnixNano(), i)
		value := fmt.Sprintf("val-%d", i)

		// Fire reads against peer nodes concurrently with the write.
		// Some of these will arrive while the coordinator is still propagating.
		done := make(chan struct{})
		go func() {
			defer close(done)
			llSet(t, coordinator, key, value)
		}()

		// Poll peer nodes immediately after kicking off the write.
		// Give the write goroutine a tiny head-start so it begins propagation.
		time.Sleep(10 * time.Millisecond)

		for _, peer := range peers {
			v, ver, found := llGet(peer, key)
			if found {
				atomic.AddInt64(&totalReads, 1)
				if ver == 0 || v == "" {
					atomic.AddInt64(&staleCount, 1)
					t.Logf("[STALE] peer=%s key=%s got_version=%d", peer, key, ver)
				}
			}
		}

		<-done // wait for write to complete before next iteration
	}

	t.Logf("Inconsistency window: %d stale reads / %d total reads across %d iterations",
		staleCount, totalReads, iterations)
	t.Log("Seeing stale reads above confirms the inconsistency window exists in the leaderless model.")
}

// TestReadFromCoordinatorAfterAck: after the coordinator returns 201, reading
// from the coordinator itself must always return consistent data.
func TestReadFromCoordinatorAfterAck(t *testing.T) {
	nodes := nodeURLs()
	key := fmt.Sprintf("ll-coord-%d", time.Now().UnixNano())
	value := "coordinator-value"

	coordinator := randomNode(nodes)
	writtenVersion := llSet(t, coordinator, key, value)

	readValue, readVersion, found := llGet(coordinator, key)
	if !found {
		t.Fatalf("coordinator %s: key %q not found after ack", coordinator, key)
	}
	if readValue != value {
		t.Errorf("coordinator %s: got value %q, want %q", coordinator, readValue, value)
	}
	if readVersion != writtenVersion {
		t.Errorf("coordinator %s: got version %d, want %d", coordinator, readVersion, writtenVersion)
	}
	t.Logf("coordinator %s consistent after ack: value=%s version=%d", coordinator, readValue, readVersion)
}

// TestReadFromPeerAfterAck: after the coordinator returns 201, all other nodes
// must also have the latest value (W=N guarantees this).
func TestReadFromPeerAfterAck(t *testing.T) {
	nodes := nodeURLs()
	key := fmt.Sprintf("ll-peer-%d", time.Now().UnixNano())
	value := "peer-after-ack"

	coordinator := randomNode(nodes)
	writtenVersion := llSet(t, coordinator, key, value)

	for _, peer := range otherNodes(nodes, coordinator) {
		readValue, readVersion, found := llGet(peer, key)
		if !found {
			t.Errorf("peer %s: key %q not found after coordinator ack", peer, key)
			continue
		}
		if readValue != value {
			t.Errorf("peer %s: got value %q, want %q", peer, readValue, value)
		}
		if readVersion != writtenVersion {
			t.Errorf("peer %s: got version %d, want %d", peer, readVersion, writtenVersion)
		}
		t.Logf("peer %s consistent after coordinator ack: version=%d", peer, readVersion)
	}
}

// TestAnyNodeCanWrite: verifies that every node can act as write coordinator
// and that after each write all nodes converge.
func TestAnyNodeCanWrite(t *testing.T) {
	nodes := nodeURLs()
	key := fmt.Sprintf("ll-anynode-%d", time.Now().UnixNano())

	for i, coordinator := range nodes {
		value := fmt.Sprintf("written-by-node%d", i)
		writtenVersion := llSet(t, coordinator, key, value)

		for _, n := range nodes {
			_, ver, found := llGet(n, key)
			if !found {
				t.Errorf("node %s: key not found after write by coordinator %s", n, coordinator)
				continue
			}
			if ver != writtenVersion {
				t.Errorf("node %s: version %d, want %d (written by %s)", n, ver, writtenVersion, coordinator)
			}
		}
		t.Logf("coordinator=%s wrote version=%d, all nodes converged", coordinator, writtenVersion)
	}
}

// TestLocalReadDuringPropagation: uses /local_read to catch nodes that haven't
// received a write yet mid-propagation. Fires a write and checks local state
// on peers immediately — this is the raw inconsistency window without the
// 50ms /get delay.
func TestLocalReadDuringPropagation(t *testing.T) {
	nodes := nodeURLs()
	const iterations = 15
	var staleCount int64

	for i := 0; i < iterations; i++ {
		coordinator := randomNode(nodes)
		peers := otherNodes(nodes, coordinator)
		key := fmt.Sprintf("ll-local-%d-%d", time.Now().UnixNano(), i)
		value := fmt.Sprintf("local-val-%d", i)

		writeDone := make(chan int64, 1)
		go func() {
			v := llSet(t, coordinator, key, value)
			writeDone <- v
		}()

		// Poll immediately — race against propagation.
		time.Sleep(5 * time.Millisecond)
		for _, peer := range peers {
			v, ver, found := llLocalRead(peer, key)
			if found && ver == 0 {
				atomic.AddInt64(&staleCount, 1)
				t.Logf("[STALE local_read] peer=%s key=%s version=%d value=%q", peer, key, ver, v)
			}
		}

		<-writeDone
	}

	t.Logf("local_read inconsistency window: %d stale / %d opportunities",
		staleCount, int64(len(nodes)-1)*iterations)
}
