package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

type planOutput struct {
	Message  string         `json:"msg"`
	Source   string         `json:"source"`
	ID       string         `json:"id"`
	Device   string         `json:"device"`
	Platform string         `json:"platform"`
	Serial   string         `json:"serial"`
	User     string         `json:"user"`
	To       string         `json:"to"`
	Rule     string         `json:"rule"`
	Status   planner.Status `json:"status"`
	Reason   string         `json:"reason,omitempty"`
	Action   app.Action     `json:"action"`
}

func writePlan(writer io.Writer, output string, results []app.Result) error {
	if output == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		for _, result := range results {
			if err := encoder.Encode(newPlanOutput(result)); err != nil {
				return fmt.Errorf("write JSON plan: %w", err)
			}
		}
		return nil
	}
	for _, result := range results {
		if _, err := fmt.Fprintf(
			writer,
			"%-10s %-10s %-8s %q -> %q  serial=%q  user=%q  rule=%q  %s\n",
			result.Status,
			result.Source,
			result.Platform,
			result.CurrentName,
			result.DesiredName,
			result.SerialNumber,
			result.User,
			result.Rule,
			result.Reason,
		); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
	}
	return nil
}

func newPlanOutput(result app.Result) planOutput {
	message := "device evaluated"
	if result.Action == app.ActionPlanned {
		message = "device rename planned"
	}
	return planOutput{
		Message:  message,
		Source:   result.Source,
		ID:       result.ID,
		Device:   result.CurrentName,
		Platform: result.Platform,
		Serial:   result.SerialNumber,
		User:     result.User,
		To:       result.DesiredName,
		Rule:     result.Rule,
		Status:   result.Status,
		Reason:   result.Reason,
		Action:   result.Action,
	}
}

func logResult(logger *slog.Logger, result app.Result) {
	attributes := []any{
		"source", result.Source,
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
	case app.ActionSubmitted:
		logger.Info("device rename submitted", attributes...)
	case app.ActionPending:
		if result.Error == "" {
			logger.Info("device rename pending", attributes...)
		} else {
			logger.Warn("device rename attempt failed", attributes...)
		}
	case app.ActionFailed:
		logger.Warn("device rename failed", attributes...)
	case app.ActionStalled:
		logger.Warn("device rename stalled", attributes...)
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
