package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
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

// SNSEnvelope unwraps the outer SNS message to get the order JSON inside
type SNSEnvelope struct {
	Message string `json:"Message"`
}

// handler is invoked once per SNS message batch.
// Each SNS record contains one order wrapped in an SNS envelope.
func handler(ctx context.Context, snsEvent events.SNSEvent) error {
	for _, record := range snsEvent.Records {
		msg := record.SNS.Message

		// 1. Parse the order directly from SNS message
		// (Lambda receives unwrapped SNS message, unlike SQS which wraps it)
		var order Order
		if err := json.Unmarshal([]byte(msg), &order); err != nil {
			log.Printf("failed to parse order: %v", err)
			continue
		}

		log.Printf("processing order %s for customer %d", order.OrderID, order.CustomerID)

		// 2. Simulate 3s payment processing — same bottleneck as before
		time.Sleep(3 * time.Second)

		order.Status = "completed"
		log.Printf("completed order %s", order.OrderID)
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
