package main

import (
	"fmt"
	"time"
)

func main() {
	m := make(map[int]int)
	done := make(chan bool, 50)

	start := time.Now()

	for g := 0; g < 50; g++ {
		go func(goroutineID int) {
			for i := 0; i < 1000; i++ {
				m[goroutineID*1000+i] = i
			}
			done <- true
		}(g)
	}
	for i := 0; i < 50; i++ {
		<-done
	}

	elapsed := time.Since(start)
	fmt.Printf("Map length: %d\n", len(m))
	fmt.Printf("Time taken: %v\n", elapsed)
}
