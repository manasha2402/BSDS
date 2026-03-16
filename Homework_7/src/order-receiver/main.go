package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
)

// --- Types ---

type Item struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

// --- Simulated Payment Bottleneck ---
// Buffered channel with capacity 1 — only 1 payment can be verified at a time.
// This is the same bottleneck the assignment describes: you can't make it faster.
var paymentSlots = make(chan struct{}, 1)

func simulatePayment() {
	paymentSlots <- struct{}{} // acquire slot (blocks if processor is busy)
	time.Sleep(3 * time.Second)
	<-paymentSlots // release slot
}

// --- Globals ---

var snsClient *sns.Client
var snsTopicARN string

// --- Handlers ---

// POST /orders/sync
// Synchronous path: verify payment inline, then respond.
// During flash sale this will timeout/queue up for customers.
func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Assign defaults
	order.OrderID = uuid.New().String()
	order.CreatedAt = time.Now()
	order.Status = "processing"

	log.Printf("[sync] received order %s for customer %d", order.OrderID, order.CustomerID)

	// Block here — customer waits the full 3 seconds
	simulatePayment()

	order.Status = "completed"
	log.Printf("[sync] completed order %s", order.OrderID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(order)
}

// POST /orders/async
// Async path: publish to SNS immediately, return 202.
// Customer gets instant acknowledgment; processing happens in background.
func asyncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Assign defaults
	order.OrderID = uuid.New().String()
	order.CreatedAt = time.Now()
	order.Status = "pending"

	log.Printf("[async] received order %s for customer %d", order.OrderID, order.CustomerID)

	// Serialize order for SNS
	payload, err := json.Marshal(order)
	if err != nil {
		http.Error(w, "failed to serialize order", http.StatusInternalServerError)
		return
	}

	// Publish to SNS — SQS subscriber will pick it up for processing
	_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
		TopicArn: aws.String(snsTopicARN),
		Message:  aws.String(string(payload)),
	})
	if err != nil {
		log.Printf("[async] failed to publish order %s: %v", order.OrderID, err)
		http.Error(w, "failed to queue order", http.StatusInternalServerError)
		return
	}

	log.Printf("[async] queued order %s", order.OrderID)

	// Return 202 immediately — don't make the customer wait
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"order_id": order.OrderID,
		"status":   "pending",
		"message":  "order received, processing in background",
	})
}

// GET /health
// Required by ECS — ALB will route traffic here to check task health.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// --- Main ---

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	snsClient = sns.NewFromConfig(cfg)

	snsTopicARN = os.Getenv("SNS_TOPIC_ARN")
	if snsTopicARN == "" {
		log.Fatal("SNS_TOPIC_ARN environment variable not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/orders/sync", syncHandler)
	http.HandleFunc("/orders/async", asyncHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("order-receiver listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
