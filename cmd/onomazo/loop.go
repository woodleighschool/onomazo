package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

type reconciler interface {
	Reconcile(context.Context, bool) ([]app.Result, error)
}

func runLoop(ctx context.Context, interval time.Duration, service reconciler, logger *slog.Logger) {
	initialCycle := true
	for {
		_ = runCycle(ctx, service, logger, initialCycle)
		initialCycle = false
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

func runCycle(ctx context.Context, service reconciler, logger *slog.Logger, initialCycle bool) error {
	started := time.Now()
	results, err := service.Reconcile(ctx, true)
	if ctx.Err() != nil {
		return err
	}
	for _, result := range results {
		logResult(logger, result, initialCycle)
	}
	attributes := []any{
		"devices", len(results),
		"renames_submitted", countAction(results, app.ActionSubmitted),
		"renames_pending", countAction(results, app.ActionPending),
		"duration", time.Since(started),
	}
	if err != nil {
		logger.ErrorContext(ctx, "reconciliation failed", append(attributes, "error", err)...)
	} else {
		logger.DebugContext(ctx, "reconciliation complete", attributes...)
	}
	return err
}

func logResult(logger *slog.Logger, result app.Result, initialCycle bool) {
	attributes := []any{
		"source", result.Source,
		"namespace", result.Namespace,
		"id", result.ID,
		"device", result.CurrentName,
		"platform", result.Platform,
		"serial", result.SerialNumber,
		"user", result.User,
		"to", result.DesiredName,
		"rule", result.Rule,
		"status", result.Status,
		"action", result.Action,
	}
	if result.Attempts != 0 {
		attributes = append(attributes, "attempts", result.Attempts)
	}
	if !result.RetryAt.IsZero() {
		attributes = append(attributes, "retry_at", result.RetryAt.Format(time.RFC3339))
	}
	if result.Error != "" {
		attributes = append(attributes, "error", result.Error)
	}
	switch result.Action {
	case app.ActionPlanned:
		logger.Info("device rename planned", attributes...)
	case app.ActionSubmitted:
		if result.Error == "" {
			logger.Info("device rename submitted", attributes...)
		} else {
			logger.Warn("device rename submitted but state update failed", attributes...)
		}
	case app.ActionPending:
		if result.Error == "" {
			if initialCycle {
				logger.Info("device rename pending", attributes...)
			} else {
				logger.Debug("device rename pending", attributes...)
			}
		} else {
			logger.Warn("device rename attempt failed", attributes...)
		}
	case app.ActionFailed:
		logger.Warn("device rename failed", attributes...)
	default:
		if result.Status == planner.StatusInvalid || result.Status == planner.StatusUnresolved {
			logger.Warn("device naming issue", append(attributes, "reason", result.Reason)...)
		} else {
			logger.Debug("device evaluated", append(attributes, "reason", result.Reason)...)
		}
	}
}

func countAction(results []app.Result, action app.Action) int {
	count := 0
	for _, result := range results {
		if result.Action == action {
			count++
		}
	}
	return count
}
