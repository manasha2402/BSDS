package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

type SearchResponse struct {
	Products   []Product `json:"products"`
	TotalFound int       `json:"total_found"`
	SearchTime string    `json:"search_time"`
}

var productStore sync.Map

var brands = []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Omega", "Sigma", "Nova", "Apex"}
var categories = []string{"Electronics", "Books", "Home", "Clothing", "Sports", "Toys", "Food", "Health", "Garden", "Auto"}
var descriptions = []string{
	"A high-quality product for everyday use.",
	"Premium grade item with excellent durability.",
	"Affordable and reliable for all needs.",
	"Top-rated by customers worldwide.",
	"Innovative design with modern features.",
}

// generateProducts creates n products and stores them in the sync.Map
func generateProducts(count int) {
	for i := 1; i <= count; i++ {
		p := Product{
			ID:          i,
			Name:        fmt.Sprintf("Product %s %d", brands[i%len(brands)], i),
			Category:    categories[i%len(categories)],
			Description: descriptions[i%len(descriptions)],
			Brand:       brands[i%len(brands)],
		}
		productStore.Store(i, p)
	}
	log.Printf("Generated %d products", count)
}

// simulateWork burns CPU to simulate expensive computation per product
func simulateWork() {
	result := 0.0
	for i := 0; i < 11000; i++ {
		result += math.Sqrt(float64(i)) * math.Sin(float64(i))
	}
}

// searchProducts checks exactly 100 products and returns up to 20 matches
func searchProducts(query string) SearchResponse {
	start := time.Now()
	query = strings.ToLower(query)

	var results []Product
	totalFound := 0
	checked := 0
	maxCheck := 100
	maxResults := 20

	productStore.Range(func(key, value interface{}) bool {
		if checked >= maxCheck {
			return false
		}
		checked++ // count EVERY product checked, not just matches

		p := value.(Product)

		// simulate fixed-cost computation (like AI inference or video processing)
		simulateWork()

		if strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.Category), query) {
			totalFound++
			if len(results) < maxResults {
				results = append(results, p)
			}
		}
		return true
	})

	elapsed := time.Since(start)

	// return empty array instead of null in JSON
	if results == nil {
		results = []Product{}
	}

	return SearchResponse{
		Products:   results,
		TotalFound: totalFound,
		SearchTime: elapsed.String(),
	}
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, `{"error": "query parameter 'q' is required"}`, http.StatusBadRequest)
		return
	}

	resp := searchProducts(q)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "healthy"}`))
}

func main() {
	log.Println("Generating 100,000 products...")
	generateProducts(100000)

	http.HandleFunc("/products/search", searchHandler)
	http.HandleFunc("/health", healthHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
