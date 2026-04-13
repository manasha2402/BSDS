package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Store struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
	region   string
}

func initS3() *s3Store {
	region := getEnvOrDefault("AWS_REGION", "us-west-2")

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET environment variable is required")
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg)
	// No Transfer Acceleration — adds latency for same-region ECS→S3 transfers
	// by routing through CloudFront edge nodes instead of the direct VPC path.
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
		u.Concurrency = 10 // 10 parallel parts for large file uploads
	})

	return &s3Store{
		client:   client,
		uploader: uploader,
		bucket:   bucket,
		region:   region,
	}
}

// upload streams r directly to S3 at the given key.
func (s *s3Store) upload(ctx context.Context, key string, r io.Reader, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	return err
}

// delete removes the object at the given S3 key.
func (s *s3Store) delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// publicURL returns the permanent public HTTPS URL for an object.
// The bucket must have a public-read bucket policy. No expiry, no signing overhead.
func (s *s3Store) publicURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
}
