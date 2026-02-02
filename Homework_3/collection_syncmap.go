package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var m sync.Map

	done := make(chan bool, 50)
	start := time.Now()

	// Spawn 50 goroutines
	for g := 0; g < 50; g++ {
		go func(goroutineID int) {
			for i := 0; i < 1000; i++ {
				m.Store(goroutineID*1000+i, i)
			}
			done <- true
		}(g)
	}

	// Wait for all to finish
	for i := 0; i < 50; i++ {
		<-done
	}

	elapsed := time.Since(start)

	// Count entries with Range
	count := 0
	m.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	fmt.Printf("Map length: %d\n", count)
	fmt.Printf("Time taken: %v\n", elapsed)
}
