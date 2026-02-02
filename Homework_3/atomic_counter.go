package main

import (
	"fmt"
	"sync"
	//"sync/atomic"
)

func main() {

	var ops uint64

	var wg sync.WaitGroup

	for range 50 {
		wg.Go(func() {
			for range 1000 {

				ops++
			}
		})
	}

	wg.Wait()

	fmt.Println("ops:", ops)
}
