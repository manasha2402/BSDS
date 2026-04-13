package main

import (
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
)

func main() {
	store := initDynamo()
	s3 := initS3()

	workerCount := 50
	if wc := os.Getenv("WORKER_COUNT"); wc != "" {
		if n, err := strconv.Atoi(wc); err == nil && n > 0 {
			workerCount = n
		}
	}
	startWorkers(workerCount, store, s3)

	mux := http.NewServeMux()

	// Health — pre-compiled byte slice, zero allocation, no DB calls.
	// ChaosArena calls this between every scenario.
	mux.HandleFunc("GET /health", handleHealth)

	mux.HandleFunc("PUT /albums/{album_id}", handlePutAlbum(store))
	mux.HandleFunc("GET /albums/{album_id}", handleGetAlbum(store))
	mux.HandleFunc("GET /albums", handleListAlbums(store))
	mux.HandleFunc("POST /albums/{album_id}/photos", handleUploadPhoto(store, s3))
	mux.HandleFunc("GET /albums/{album_id}/photos/{photo_id}", handleGetPhoto(store, s3))
	mux.HandleFunc("DELETE /albums/{album_id}/photos/{photo_id}", handleDeletePhoto(store, s3))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, recoveryMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

// healthBody is pre-compiled to avoid JSON encoding overhead on every probe.
var healthBody = []byte(`{"status":"ok"}`)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(healthBody) //nolint:errcheck
}

// recoveryMiddleware catches any handler panic and returns a proper JSON 500
// instead of dropping the connection (which causes ALB to return 502).
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC %s %s: %v\n%s", r.Method, r.URL.Path, err, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal error"}`)) //nolint:errcheck
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
