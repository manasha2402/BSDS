package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
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

func mapHandler(c *gin.Context) {
	chunkURL := c.Query("chunk_url")
	if chunkURL == "" {
		c.JSON(400, gin.H{"error": "Missing chunk_url"})
		return
	}

	bucket, key := parseS3URL(chunkURL)
	text, err := downloadFromS3(bucket, key)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	text = strings.ToLower(text)
	regex := regexp.MustCompile(`[a-z]+`)
	words := regex.FindAllString(text, -1)

	wordCounts := make(map[string]int)
	for _, word := range words {
		wordCounts[word]++
	}

	jsonData, err := json.Marshal(wordCounts)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	chunkName := strings.Split(key, "/")[1]
	chunkName = strings.TrimSuffix(chunkName, ".txt")
	resultKey := fmt.Sprintf("mapped/%s_result.json", chunkName)

	err = uploadToS3(bucket, resultKey, string(jsonData))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":          "success",
		"words_processed": len(words),
		"unique_words":    len(wordCounts),
		"result_url":      fmt.Sprintf("s3://%s/%s", bucket, resultKey),
	})
}

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"service": "Mapper", "status": "running"})
	})

	r.GET("/map", mapHandler)

	fmt.Println("🗺️  Mapper service on :8080")
	r.Run(":8080")
}
