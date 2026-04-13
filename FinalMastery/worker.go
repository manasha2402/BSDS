package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"time"
)

// jobQueue is the in-process channel between POST handlers and worker goroutines.
// Buffer of 10 000 means Submit never blocks in practice.
var jobQueue = make(chan PhotoJob, 10000)

func startWorkers(count int, store *dynamoStore, s3 *s3Store) {
	for i := 0; i < count; i++ {
		go runWorker(store, s3)
	}
	log.Printf("started %d worker goroutines", count)
}

func runWorker(store *dynamoStore, s3 *s3Store) {
	for job := range jobQueue {
		processJob(job, store, s3)
	}
}

func processJob(job PhotoJob, store *dynamoStore, s3 *s3Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := s3.upload(ctx, job.S3Key, bytes.NewReader(job.FileBytes), job.ContentType); err != nil {
		log.Printf("S3 upload failed for %s: %v", job.PhotoID, err)
		if err2 := store.UpdatePhotoStatus(ctx, job.PhotoID, "failed"); err2 != nil {
			if !errors.Is(err2, ErrPhotoDeleted) {
				log.Printf("DB update failed for %s: %v", job.PhotoID, err2)
			}
		}
		return
	}

	if err := store.UpdatePhotoStatus(ctx, job.PhotoID, "completed"); err != nil {
		if errors.Is(err, ErrPhotoDeleted) {
			// Photo was deleted while we were uploading (S9). Clean up the
			// orphaned S3 object so the URL returns non-200.
			s3.delete(context.Background(), job.S3Key) //nolint:errcheck
			return
		}
		log.Printf("DB update failed for %s: %v", job.PhotoID, err)
	}
}
