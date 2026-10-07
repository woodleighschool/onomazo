package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

type reconciliationReport struct {
	Devices []app.Result  `json:"devices"`
	Summary reportSummary `json:"summary"`
	Error   string        `json:"error,omitempty"`
}

type reportSummary struct {
	Total    int                    `json:"total"`
	Statuses map[planner.Status]int `json:"statuses"`
	Actions  map[app.Action]int     `json:"actions"`
}

type reportError struct {
	command string
	cause   error
}

func (e *reportError) Error() string {
	return e.command + ": " + strings.SplitN(e.cause.Error(), "\n", 2)[0]
}
func (e *reportError) Unwrap() error { return e.cause }

func writeReport(writer io.Writer, jsonOutput, includeUnchanged, apply bool, results []app.Result, runErr error) error {
	if results == nil {
		return nil
	}
	report := reconciliationReport{
		Devices: make([]app.Result, 0, len(results)),
		Summary: reportSummary{Total: len(results), Statuses: make(map[planner.Status]int), Actions: make(map[app.Action]int)},
	}
	for _, result := range results {
		report.Summary.Statuses[result.Status]++
		if result.Action != "" {
			report.Summary.Actions[result.Action]++
		}
		if jsonOutput || includeUnchanged || result.Status != planner.StatusUnchanged || result.Error != "" {
			report.Devices = append(report.Devices, result)
		}
	}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	if jsonOutput {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(report)
	}

	style := newTextStyle(writer)
	var text strings.Builder
	heading := "Naming plan"
	if apply {
		heading = "Apply results"
	}
	fmt.Fprintf(&text, "%s\n", style.heading(heading))
	for _, result := range report.Devices {
		writeDevice(&text, style, result, apply)
	}
	statuses := report.Summary.Statuses
	fmt.Fprintf(&text, "\n%s %d total, %d rename, %d unchanged, %d excluded, %d unmanaged, %d invalid, %d unresolved\n",
		style.paint("Devices:", color.Bold), report.Summary.Total, statuses[planner.StatusRename], statuses[planner.StatusUnchanged],
		statuses[planner.StatusExcluded], statuses[planner.StatusUnmanaged], statuses[planner.StatusInvalid], statuses[planner.StatusUnresolved])
	if apply {
		actions := report.Summary.Actions
		fmt.Fprintf(&text, "%s %d submitted, %d pending, %d failed, %d not submitted\n",
			style.paint("Requests:", color.Bold), actions[app.ActionSubmitted], actions[app.ActionPending], actions[app.ActionFailed], actions[app.ActionPlanned])
	} else {
		text.WriteString("No rename requests submitted. The plan describes naming policy, not request eligibility.\n")
	}
	if runErr != nil {
		text.WriteString("Reconciliation incomplete; reported request outcomes remain valid.\n")
	}
	_, err := io.WriteString(writer, text.String())
	return err
}

func writeDevice(text *strings.Builder, style textStyle, result app.Result, apply bool) {
	label := strings.ToUpper(string(result.Status))
	var outcome string
	if result.Status == planner.StatusRename {
		switch result.Action {
		case app.ActionSubmitted:
			label, outcome = "SUBMITTED", "Request accepted; awaiting inventory confirmation."
		case app.ActionPending:
			label, outcome = "PENDING", "Request already accepted; awaiting inventory confirmation."
			if !result.RetryAt.IsZero() {
				label = "RETRY"
				outcome = "Eligible after " + result.RetryAt.UTC().Format(time.RFC3339) + " on a subsequent apply/run cycle."
			}
		case app.ActionFailed:
			label, outcome = "FAILED", "No further submissions for this intention."
		case app.ActionPlanned, "":
			label = "PLANNED"
			if apply {
				label = "NOT SUBMITTED"
			}
		}
	}
	name := strconv.Quote(result.CurrentName)
	if result.DesiredName != "" && result.DesiredName != result.CurrentName {
		name += " → " + strconv.Quote(result.DesiredName)
	}
	fmt.Fprintf(text, "\n%s: %s\n  Device: %s/%s/%s", style.heading(name), style.paint(strings.ToLower(label), labelColour(label)), humanText(result.Source), humanText(result.Namespace), humanText(result.ID))
	if result.SerialNumber != "" {
		fmt.Fprintf(text, "\n  Serial: %s", humanText(result.SerialNumber))
	}
	if result.Platform != "" {
		fmt.Fprintf(text, "\n  Platform: %s", humanText(result.Platform))
	}
	text.WriteByte('\n')
	if result.Rule != "" {
		fmt.Fprintf(text, "  Rule: %s\n", humanText(result.Rule))
	}
	if result.User != "" {
		fmt.Fprintf(text, "  User: %s\n", humanText(result.User))
	}
	if result.Reason != "" && result.Reason != "name differs" && result.Reason != "name already matches" {
		fmt.Fprintf(text, "  Reason: %s\n", humanText(result.Reason))
	}
	if outcome != "" {
		mark, attribute := "i", color.Faint
		switch result.Action {
		case app.ActionSubmitted:
			mark, attribute = "✓", color.FgHiGreen
		case app.ActionPending:
			mark, attribute = "–", color.FgHiYellow
		case app.ActionFailed, app.ActionPlanned, "":
		}
		fmt.Fprintf(text, "  %s %s\n", style.paint(mark, attribute), outcome)
	}
	if result.Attempts != 0 {
		fmt.Fprintf(text, "  Attempts: %d\n", result.Attempts)
	}
	if result.Error != "" {
		fmt.Fprintf(text, "  %s %s\n", style.paint("✗", color.FgHiRed), humanText(result.Error))
	}
}

func labelColour(label string) color.Attribute {
	switch label {
	case "SUBMITTED":
		return color.FgHiGreen
	case "FAILED", "INVALID":
		return color.FgHiRed
	case "PLANNED", "PENDING", "RETRY", "NOT SUBMITTED", "UNRESOLVED":
		return color.FgHiYellow
	}
	return color.Faint
}

// humanText escapes the characters in one value that could reshape a report
// or control a terminal. Printable text, including non-ASCII letters, is unchanged.
func humanText(value string) string {
	var text strings.Builder
	for _, r := range value {
		if strconv.IsPrint(r) {
			text.WriteRune(r)
		} else {
			quoted := strconv.QuoteRune(r)
			text.WriteString(quoted[1 : len(quoted)-1])
		}
	}
	return text.String()
}
