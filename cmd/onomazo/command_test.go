package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/app"
)

func TestRunLoopCancelsInFlightReconciliation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	service := blockingReconciler{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runLoop(ctx, time.Hour, service, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("runLoop() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runLoop() did not stop after cancellation")
	}
}

type blockingReconciler struct {
	started chan<- struct{}
}

func (r blockingReconciler) Reconcile(ctx context.Context, _ bool) ([]app.Result, error) {
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
