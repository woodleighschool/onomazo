package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
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
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo}))
	go func() {
		runLoop(ctx, time.Hour, service, logger)
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runLoop() did not stop after cancellation")
	}
	if strings.Contains(output.String(), "reconciliation failed") {
		t.Errorf("shutdown logged a reconciliation failure: %s", output.String())
	}
}

func TestRunLoopLogsPendingRenamesAtInfoOnlyOnFirstCycle(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	service := &cancellingReconciler{cancel: cancel, cancelAfter: 3}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runLoop(ctx, 0, service, logger)

	var pendingLevels []string
	var summaryLevels []string
	decoder := json.NewDecoder(&output)
	for {
		var record struct {
			Level   string `json:"level"`
			Message string `json:"msg"`
		}
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode log: %v", err)
		}
		if record.Message == "device rename pending" {
			pendingLevels = append(pendingLevels, record.Level)
		}
		if record.Message == "reconciliation complete" {
			summaryLevels = append(summaryLevels, record.Level)
		}
	}
	if got, want := len(pendingLevels), 2; got != want {
		t.Fatalf("pending log records = %d, want %d: %s", got, want, output.String())
	}
	if got, want := pendingLevels[0], "INFO"; got != want {
		t.Errorf("first pending log level = %q, want %q", got, want)
	}
	if got, want := pendingLevels[1], "DEBUG"; got != want {
		t.Errorf("later pending log level = %q, want %q", got, want)
	}
	if got, want := summaryLevels, []string{"DEBUG", "DEBUG"}; !slices.Equal(got, want) {
		t.Errorf("summary log levels = %v, want %v", got, want)
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

type cancellingReconciler struct {
	cancel      context.CancelFunc
	cancelAfter int
	calls       int
}

func (r *cancellingReconciler) Reconcile(context.Context, bool) ([]app.Result, error) {
	r.calls++
	if r.calls == r.cancelAfter {
		r.cancel()
	}
	return []app.Result{{Action: app.ActionPending}}, nil
}
