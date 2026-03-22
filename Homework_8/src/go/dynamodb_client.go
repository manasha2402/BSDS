package openapi

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

var dynamoClient *dynamodb.Client

func InitDynamoDB() {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(os.Getenv("AWS_REGION")),
	)
	if err != nil {
		log.Fatalf("ERROR loading AWS config: %v", err)
	}
	dynamoClient = dynamodb.NewFromConfig(cfg)
	log.Println("DynamoDB client initialized")
}

func GetDynamoDB() *dynamodb.Client {
	return dynamoClient
}
