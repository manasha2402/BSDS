package main

import (
	"fmt"
	"sync"
	"time"
)

type SafeMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func main() {
	SafeMap := SafeMap{
		m: make(map[int]int),
	}
	done := make(chan bool, 50)
	start := time.Now()

	for g := 0; g < 50; g++ {
		go func(goroutineID int) {
			for i := 0; i < 1000; i++ {
				SafeMap.mu.Lock()
				SafeMap.m[goroutineID*1000+i] = i
				SafeMap.mu.Unlock()
			}
			done <- true
		}(g)
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	elapsed := time.Since(start)
	SafeMap.mu.RLock()
	fmt.Printf("Map length: %d\n", len(SafeMap.m))
	SafeMap.mu.RUnlock()
	fmt.Printf("Time taken: %v\n", elapsed)
}
