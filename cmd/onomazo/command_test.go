package main

import (
	"context"
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
	done := make(chan struct{})
	go func() {
		runLoop(ctx, time.Hour, service, slog.New(slog.DiscardHandler))
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
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
