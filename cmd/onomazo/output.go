package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

type reconciliationReport struct {
	Devices []app.Result `json:"devices,omitzero"`
	Error   string       `json:"error,omitempty"`
}

func writeReport(writer io.Writer, output string, includeUnchanged, apply bool, results []app.Result, runErr error) error {
	report := reconciliationReport{Devices: results}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	if output == "json" {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(report)
	}
	if results == nil && runErr != nil {
		_, err := fmt.Fprintln(writer, "No device results available.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(
		table,
		"STATUS\tACTION\tSOURCE\tPLATFORM\tCURRENT\tDESIRED\tSERIAL\tUSER\tRULE\tREASON\tERROR",
	); err != nil {
		return fmt.Errorf("write plan header: %w", err)
	}
	for _, result := range results {
		if !includeUnchanged && result.Status == planner.StatusUnchanged && result.Error == "" {
			continue
		}
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			result.Status,
			result.Action,
			result.Source,
			result.Platform,
			result.CurrentName,
			result.DesiredName,
			result.SerialNumber,
			result.User,
			result.Rule,
			result.Reason,
			result.Error,
		); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("flush plan: %w", err)
	}
	counts := make(map[planner.Status]int)
	for _, result := range results {
		counts[result.Status]++
	}
	_, err := fmt.Fprintf(
		writer,
		"\nDevice summary: %d total, %d rename, %d unchanged, %d excluded, %d unmanaged, %d invalid, %d unresolved\n",
		len(results),
		counts[planner.StatusRename],
		counts[planner.StatusUnchanged],
		counts[planner.StatusExcluded],
		counts[planner.StatusUnmanaged],
		counts[planner.StatusInvalid],
		counts[planner.StatusUnresolved],
	)
	if err != nil {
		return err
	}
	if apply {
		_, err = fmt.Fprintf(writer, "Renames: %d submitted, %d pending, %d failed\n",
			countAction(results, app.ActionSubmitted), countAction(results, app.ActionPending), countAction(results, app.ActionFailed))
	}
	return err
}
