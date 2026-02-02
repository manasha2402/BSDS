package main

import (
	"fmt"
	"runtime"
	"time"
)

const N = 1_000_000

func burnCPU() {
	x := 0
	for i := 0; i < 100; i++ {
		x += i * i
	}
	_ = x
}

func singleThread() time.Duration {
	runtime.GOMAXPROCS(1)

	ch := make(chan struct{})

	go func() {
		for i := 0; i < N; i++ {
			burnCPU() // 👈 added
			<-ch
			ch <- struct{}{}
		}
	}()

	start := time.Now()
	for i := 0; i < N; i++ {
		burnCPU()
		ch <- struct{}{}
		<-ch
	}
	return time.Since(start)
}

func multiThread() time.Duration {
	runtime.GOMAXPROCS(0) // 0 = use all available CPUs

	ch := make(chan struct{})

	go func() {
		for i := 0; i < N; i++ {
			burnCPU()
			<-ch
			ch <- struct{}{}
		}
	}()

	start := time.Now()
	for i := 0; i < N; i++ {
		burnCPU()
		ch <- struct{}{}
		<-ch
	}
	return time.Since(start)
}

func main() {
	single := singleThread()
	multi := multiThread()

	fmt.Printf("Single-threaded: %v\n", single)
	fmt.Printf("Multi-threaded: %v\n", multi)
	fmt.Printf("Ratio: %.2fx\n", float64(single)/float64(multi))
}
