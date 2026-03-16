package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

// SNSEnvelope unwraps the outer SNS message before we can read the order inside
type SNSEnvelope struct {
	Message string `json:"Message"`
}

// sem is a buffered channel used as a semaphore.
// It limits how many orders can be processed at the same time.
// Capacity is set to NUM_WORKERS at startup.
var sem chan struct{}

// --- Worker ---

// processOrder handles a single SQS message:
// parse → simulate payment → delete from queue.
// It always releases the semaphore slot when done via defer.
func processOrder(msg sqstypes.Message, client *sqs.Client, queueURL string) {
	defer func() { <-sem }() // release slot when done, even if we error out

	// 1. Unwrap the SNS envelope to get the raw order JSON inside
	var env SNSEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &env); err != nil {
		log.Printf("failed to unwrap SNS envelope: %v", err)
		return
	}

	// 2. Parse the actual order from the unwrapped message
	var order Order
	if err := json.Unmarshal([]byte(env.Message), &order); err != nil {
		log.Printf("failed to parse order: %v", err)
		return
	}

	log.Printf("processing order %s for customer %d", order.OrderID, order.CustomerID)

	// 3. Simulate 3s payment verification — this is the bottleneck we can't speed up
	time.Sleep(3 * time.Second)

	order.Status = "completed"
	log.Printf("completed order %s", order.OrderID)

	// 4. Delete from SQS — CRITICAL: without this the message reappears after
	// the visibility timeout and gets processed again
	if _, err := client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	}); err != nil {
		log.Printf("failed to delete message for order %s: %v", order.OrderID, err)
	}
}

// startWorker continuously polls SQS for new orders and dispatches them.
// It blocks before spawning a goroutine until a semaphore slot is free,
// so we never exceed NUM_WORKERS concurrent goroutines.
func startWorker(ctx context.Context, client *sqs.Client, queueURL string) {
	log.Println("worker started, polling SQS...")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Long poll: waits up to 20s for messages, returns up to 10 at once.
		// This is cheaper than short polling and reduces empty responses.
		result, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   30, // hides message from other consumers while processing
		})
		if err != nil {
			log.Printf("error receiving messages: %v", err)
			time.Sleep(1 * time.Second) // back off briefly before retrying
			continue
		}

		for _, msg := range result.Messages {
			// Acquire a slot before spawning — blocks here if all workers are busy.
			// This prevents unbounded goroutine growth during a flash sale spike.
			sem <- struct{}{}
			go processOrder(msg, client, queueURL)
		}
	}
}

// --- Main ---

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	client := sqs.NewFromConfig(cfg)

	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		log.Fatal("SQS_QUEUE_URL environment variable not set")
	}

	// Phase 3: NUM_WORKERS=1
	// Phase 5: bump to 5, 20, 100 to observe queue drain behavior
	numWorkers := 100
	if v := os.Getenv("NUM_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			numWorkers = n
		}
	}

	sem = make(chan struct{}, numWorkers)

	log.Printf("starting processor with %d worker(s)", numWorkers)
	startWorker(context.Background(), client, queueURL)
}
