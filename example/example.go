package main

import (
	"fmt"
	"time"

	"github.com/fiber/cron"
)

// Example demonstrates how to use the cron package to schedule and manage jobs.
func main() {
	scheduler := cron.NewScheduler()

	// Add jobs
	job1, err := scheduler.AddJob("* * * * *", func() {
		fmt.Println("This job runs every minute")
	})
	if err != nil {
		fmt.Printf("Error adding job1: %v\n", err)
		return
	}

	job2, err := scheduler.AddJob("0 * * * *", func() {
		fmt.Println("This job runs every hour")
	})
	if err != nil {
		fmt.Printf("Error adding job2: %v\n", err)
		return
	}

	job3, err := scheduler.AddJob("0 0 * * *", func() {
		fmt.Println("This job runs every day at midnight")
	})
	if err != nil {
		fmt.Printf("Error adding job3: %v\n", err)
		return
	}

	// Simulate some job modifications
	time.Sleep(5 * time.Second)
	err = scheduler.UpdateJob(job2, "*/30 * * * *")
	if err != nil {
		fmt.Printf("Error updating job2: %v\n", err)
	}

	time.Sleep(5 * time.Second)
	scheduler.RemoveJob(job3)

	// Try to update the removed job (this should fail)
	err = scheduler.UpdateJob(job3, "0 12 * * *")
	if err != nil {
		fmt.Printf("Expected error when updating removed job: %v\n", err)
	}

	// Let it run for a while
	time.Sleep(50 * time.Second)

	// Stop the scheduler
	scheduler.Stop()

	// Output:
	// This job runs every minute
	// This job runs every minute
	// Expected error when updating removed job: job not found in scheduler
	// This job runs every minute
	// This job runs every 30 minutes
	// This job runs every minute
}
