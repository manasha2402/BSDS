package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

func unbufferedWrite() time.Duration {
	f, err := os.Create("unbuffered.txt")
	if err != nil {
		fmt.Println("Error:", err)
	}
	start := time.Now()
	for i := 0; i < 100000; i++ {
		f.Write([]byte("Line " + fmt.Sprintf("%d\n", i)))
	}
	f.Close()
	return time.Since(start)
}

func bufferedWrite() time.Duration {
	f, err := os.Create("buffered.txt")
	if err != nil {
		fmt.Println("Error:", err)
	}
	w := bufio.NewWriter(f)
	start := time.Now()
	for i := 0; i < 100000; i++ {
		w.WriteString("Line " + fmt.Sprintf("%d\n", i))
	}
	w.Flush()
	f.Close()
	return time.Since(start)
}

func main() {
	u := unbufferedWrite()
	b := bufferedWrite()
	fmt.Printf("Unbuffered: %v\n", u)
	fmt.Printf("Buffered: %v\n", b)
}
