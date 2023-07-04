package main

import (
	"fmt"
	"time"
)

const workerPoolSize = 10

func main() {
	tasks := make(chan string, workerPoolSize)

	// Create a ticker that fires every 100 milliseconds (10 times per second)
	ticker := time.NewTicker(1000 * time.Millisecond)

	// Create workers
	for i := 0; i < workerPoolSize; i++ {
		go worker(tasks)
	}

	// Feed workers from ticker
	for range ticker.C {
		tasks <- "hi"
	}
}

func worker(tasks chan string) {
	for {
		task := <-tasks
		fmt.Println(task)
	}
}
