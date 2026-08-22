package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

type planOutput struct {
	Message   string         `json:"msg"`
	Source    string         `json:"source"`
	Namespace string         `json:"namespace"`
	ID        string         `json:"id"`
	Device    string         `json:"device"`
	Platform  string         `json:"platform"`
	Serial    string         `json:"serial"`
	User      string         `json:"user"`
	To        string         `json:"to"`
	Rule      string         `json:"rule"`
	Status    planner.Status `json:"status"`
	Reason    string         `json:"reason,omitempty"`
	Action    app.Action     `json:"action"`
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
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(
		table,
		"STATUS\tSOURCE\tPLATFORM\tCURRENT\tDESIRED\tSERIAL\tUSER\tRULE\tREASON",
	); err != nil {
		return fmt.Errorf("write plan header: %w", err)
	}
	for _, result := range results {
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
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
	if err := table.Flush(); err != nil {
		return fmt.Errorf("flush plan: %w", err)
	}
	return nil
}

func newPlanOutput(result app.Result) planOutput {
	message := "device evaluated"
	if result.Action == app.ActionPlanned {
		message = "device rename planned"
	}
	return planOutput{
		Message:   message,
		Source:    result.Source,
		Namespace: result.Namespace,
		ID:        result.ID,
		Device:    result.CurrentName,
		Platform:  result.Platform,
		Serial:    result.SerialNumber,
		User:      result.User,
		To:        result.DesiredName,
		Rule:      result.Rule,
		Status:    result.Status,
		Reason:    result.Reason,
		Action:    result.Action,
	}
}
