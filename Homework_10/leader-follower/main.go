package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Config holds everything the node needs to know about itself
// and the cluster. All values come from environment variables
// so Docker Compose can configure each container differently.
type Config struct {
	Role  string   // "leader" or "follower"
	Port  string   // port this node listens on
	Peers []string // list of peer URLs (followers if leader, leader if follower)
	W     int      // write quorum
	R     int      // read quorum
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

	// Public API endpoints
	mux.HandleFunc("/set", handleSet)
	mux.HandleFunc("/get", handleGet)

	// Internal endpoint — used for testing inconsistency windows
	// and for leader-to-follower replication calls
	mux.HandleFunc("/local_read", handleLocalRead)

	// Internal endpoint — leader calls this on followers during replication
	mux.HandleFunc("/internal/set", handleInternalSet)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("[%s] listening on %s (W=%d, R=%d, peers=%v)",
		cfg.Role, addr, cfg.W, cfg.R, cfg.Peers)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// loadConfig reads all configuration from environment variables.
//
//	ROLE    = "leader" | "follower"
//	PORT    = "8080"
//	PEERS   = "http://follower1:8080,http://follower2:8080,..."
//	W       = write quorum  (e.g. 1, 3, or 5)
//	R       = read quorum   (e.g. 1, 3, or 5)
func loadConfig() Config {
	role := getEnv("ROLE", "follower")
	port := getEnv("PORT", "8080")
	w := getEnvInt("W", 5)
	r := getEnvInt("R", 1)

	var peers []string
	if p := os.Getenv("PEERS"); p != "" {
		for _, addr := range strings.Split(p, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				peers = append(peers, addr)
			}
		}
	}

	return Config{Role: role, Port: port, Peers: peers, W: w, R: r}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
