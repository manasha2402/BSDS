package main

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
)

var s3Client *s3.S3

func init() {
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	}))
	s3Client = s3.New(sess)
}

func parseS3URL(url string) (bucket, key string) {
	url = strings.TrimPrefix(url, "s3://")
	parts := strings.SplitN(url, "/", 2)
	bucket = parts[0]
	if len(parts) > 1 {
		key = parts[1]
	}
	return
}

func downloadFromS3(bucket, key string) (string, error) {
	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", err
	}
	defer result.Body.Close()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func uploadToS3(bucket, key, content string) error {
	_, err := s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	})
	return err
}

func splitHandler(c *gin.Context) {
	inputURL := c.Query("input_url")
	if inputURL == "" {
		c.JSON(400, gin.H{"error": "Missing input_url"})
		return
	}

	bucket, key := parseS3URL(inputURL)
	text, err := downloadFromS3(bucket, key)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	words := strings.Fields(text)
	totalWords := len(words)
	numChunks := 3
	chunkSize := int(math.Ceil(float64(totalWords) / float64(numChunks)))

	chunkURLs := []string{}
	for i := 0; i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > totalWords {
			end = totalWords
		}

		chunkWords := words[start:end]
		chunkText := strings.Join(chunkWords, " ")

		chunkKey := fmt.Sprintf("chunks/chunk_%d.txt", i)
		err := uploadToS3(bucket, chunkKey, chunkText)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		chunkURLs = append(chunkURLs, fmt.Sprintf("s3://%s/%s", bucket, chunkKey))
	}

	c.JSON(200, gin.H{
		"status":      "success",
		"total_words": totalWords,
		"num_chunks":  numChunks,
		"chunk_urls":  chunkURLs,
	})
}

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"service": "Splitter", "status": "running"})
	})

	r.GET("/split", splitHandler)

	fmt.Println("🔨 Splitter service on :8080")
	r.Run(":8080")
}
