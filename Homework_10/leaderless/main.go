package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// Config holds everything a node needs to know about itself
// and the cluster. In the leaderless model every node is identical —
// there is no role distinction, only a list of peers.
type Config struct {
	NodeID string   // a human-readable name for logging (e.g. "node1")
	Port   string   // port this node listens on
	Peers  []string // URLs of all OTHER nodes in the cluster
}

var (
	cfg   Config
	store *Store
)

func main() {
	cfg = loadConfig()
	store = NewStore()

	mux := http.NewServeMux()

	// Health check for ALB
	mux.HandleFunc("/health", handleHealth)

	// Public API — any node can receive reads or writes from clients.
	mux.HandleFunc("/set", handleSet)
	mux.HandleFunc("/get", handleGet)

	// Sneaky test endpoint — returns raw local value, no coordination.
	mux.HandleFunc("/local_read", handleLocalRead)

	// Internal endpoint — the Write Coordinator calls this on peer nodes
	// during write propagation. Not meant for clients.
	mux.HandleFunc("/internal/set", handleInternalSet)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("[%s] leaderless node listening on %s (peers=%v)",
		cfg.NodeID, addr, cfg.Peers)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// loadConfig reads configuration from environment variables.
//
//	NODE_ID = "node1"  (just for logging)
//	PORT    = "8080"
//	PEERS   = "http://node2:8080,http://node3:8080,..."
//
// Note: no ROLE, W, or R env vars here.
// The leaderless implementation is fixed at W=N, R=1
// as required by the assignment.
func loadConfig() Config {
	nodeID := getEnv("NODE_ID", "node")
	port := getEnv("PORT", "8080")

	var peers []string
	if p := os.Getenv("PEERS"); p != "" {
		for _, addr := range strings.Split(p, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				peers = append(peers, addr)
			}
		}
	}

	return Config{NodeID: nodeID, Port: port, Peers: peers}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
