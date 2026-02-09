package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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

func reduceHandler(c *gin.Context) {
	resultURLs := c.Query("result_urls")
	if resultURLs == "" {
		c.JSON(400, gin.H{"error": "Missing result_urls"})
		return
	}

	urls := strings.Split(resultURLs, ",")
	finalCounts := make(map[string]int)

	for _, url := range urls {
		url = strings.TrimSpace(url)
		bucket, key := parseS3URL(url)

		jsonData, err := downloadFromS3(bucket, key)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		var counts map[string]int
		err = json.Unmarshal([]byte(jsonData), &counts)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		for word, count := range counts {
			finalCounts[word] += count
		}
	}

	// Get top 20
	type wordCount struct {
		word  string
		count int
	}

	pairs := []wordCount{}
	for word, count := range finalCounts {
		pairs = append(pairs, wordCount{word, count})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})

	top20 := make(map[string]int)
	for i := 0; i < 20 && i < len(pairs); i++ {
		top20[pairs[i].word] = pairs[i].count
	}

	finalResult := map[string]interface{}{
		"total_unique_words": len(finalCounts),
		"top_20_words":       top20,
		"all_counts":         finalCounts,
	}

	finalJSON, err := json.MarshalIndent(finalResult, "", "  ")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	bucket, _ := parseS3URL(urls[0])
	finalKey := "final_result.json"
	err = uploadToS3(bucket, finalKey, string(finalJSON))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"status":             "success",
		"total_unique_words": len(finalCounts),
		"top_20_words":       top20,
		"final_url":          fmt.Sprintf("s3://%s/%s", bucket, finalKey),
	})
}

func main() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"service": "Reducer", "status": "running"})
	})

	r.GET("/reduce", reduceHandler)

	fmt.Println("�� Reducer service on :8080")
	r.Run(":8080")
}
