package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/qw145/hotelbooking/helper"
	"github.com/qw145/hotelbooking/model"
)

func TestPool(t *testing.T) {
	helper.Log = helper.NewLogger(nil)
	maxWorker := 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewPool(maxWorker, 100)
	pool.Run(ctx)

	for i := 1; i <= 4; i++ {
		pool.Collector <- &EmailSender{Log: helper.Log, Booking: &model.Booking{}}
	}

	go func() {
		close(pool.Collector)
		pool.Wg.Wait()

		close(pool.Results)
	}()

	for res := range pool.Results {
		if res.Err != nil {
			fmt.Printf("Job failed: %v\n", res.Err)
		}
	}

	// Final check on how the lifecycle completed
	if ctx.Err() != nil {
		t.Fatalf("Execution lifecycle finished with context state: %v\n", ctx.Err())
	}
}
