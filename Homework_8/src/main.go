package main

import (
	"log"

	sw "github.com/GIT_USER_ID/GIT_REPO_ID/go"
)

func main() {
	// Initialize MySQL database connection and schema
	err := sw.InitDB()
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}

	// Initialize DynamoDB client
	sw.InitDynamoDB()
	// Initialize API handlers
	routes := sw.ApiHandleFunctions{
		ShoppingCartAPI:       sw.ShoppingCartAPI{},
		DynamoShoppingCartAPI: sw.DynamoShoppingCartAPI{}, // for DynamoDB-based shopping cart handlers
	}

	log.Printf("Server started")

	// Create router with registered routes
	router := sw.NewRouter(routes)

	// Start HTTP server on port 8080
	log.Fatal(router.Run(":8080"))
}
