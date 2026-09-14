package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

func TestWriteHumanPlanUsesAlignedColumns(t *testing.T) {
	t.Parallel()
	results := []app.Result{
		{
			Source:       "intune",
			ID:           "device-1",
			SerialNumber: "SERIAL-1",
			Platform:     "ios",
			CurrentName:  "OLD",
			DesiredName:  "NEW",
			User:         "unit@example.invalid",
			Rule:         "fixture-rule",
			Status:       planner.StatusRename,
			Reason:       "name differs"},
		{
			Source:       "jamf",
			ID:           "device-2",
			SerialNumber: "SERIAL-2",
			Platform:     "macos",
			CurrentName:  "LONG-CURRENT-NAME",
			DesiredName:  "TARGET",
			User:         "other@example.invalid",
			Rule:         "fallback",
			Status:       planner.StatusUnmanaged,
			Reason:       "no naming rule matched"},
	}
	var output bytes.Buffer
	if err := writeReport(&output, "text", false, false, results, nil); err != nil {
		t.Fatalf("writePlan() error = %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if got, want := len(lines), 5; got != want {
		t.Fatalf("output lines = %d, want %d: %q", got, want, output.String())
	}
	columns := []struct {
		header string
		first  string
		second string
	}{
		{header: "STATUS", first: "rename", second: "unmanaged"},
		{header: "SOURCE", first: "intune", second: "jamf"},
		{header: "PLATFORM", first: "ios", second: "macos"},
		{header: "CURRENT", first: "OLD", second: "LONG-CURRENT-NAME"},
		{header: "DESIRED", first: "NEW", second: "TARGET"},
		{header: "SERIAL", first: "SERIAL-1", second: "SERIAL-2"},
		{header: "USER", first: "unit@example.invalid", second: "other@example.invalid"},
		{header: "RULE", first: "fixture-rule", second: "fallback"},
		{header: "REASON", first: "name differs", second: "no naming rule matched"},
	}
	for _, column := range columns {
		wantStart := strings.Index(lines[0], column.header)
		if wantStart < 0 {
			t.Fatalf("header %q missing from %q", column.header, lines[0])
		}
		if got := strings.Index(lines[1], column.first); got != wantStart {
			t.Errorf("first-row %s starts at %d, want %d: %q", column.header, got, wantStart, lines[1])
		}
		if got := strings.Index(lines[2], column.second); got != wantStart {
			t.Errorf("second-row %s starts at %d, want %d: %q", column.header, got, wantStart, lines[2])
		}
	}
	if strings.Contains(output.String(), "\t") {
		t.Errorf("human plan contains unexpanded tabs: %q", output.String())
	}
}

func TestWriteHumanPlanIncludesUnchangedOnlyWithAll(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		var output bytes.Buffer
		results := outputFixture()
		if err := writeReport(&output, "text", all, false, results, nil); err != nil {
			t.Fatal(err)
		}
		text := output.String()
		for _, result := range results {
			want := all || result.Status != planner.StatusUnchanged
			if got := strings.Contains(text, result.SerialNumber); got != want {
				t.Errorf("all %t: plan contains %s = %t, want %t:\n%s", all, result.Status, got, want, text)
			}
		}
		if !strings.Contains(text, "Device summary: 6 total, 1 rename, 1 unchanged, 1 excluded, 1 unmanaged, 1 invalid, 1 unresolved") {
			t.Errorf("all %t: plan missing complete summary:\n%s", all, text)
		}
	}
}

func outputFixture() []app.Result {
	return []app.Result{
		{SerialNumber: "SERIAL-1", Status: planner.StatusRename, CurrentName: "OLD", DesiredName: "NEW", Reason: "name differs", Action: app.ActionPlanned},
		{SerialNumber: "SERIAL-2", Status: planner.StatusUnchanged, CurrentName: "CURRENT", DesiredName: "CURRENT", Reason: "name already matches"},
		{SerialNumber: "SERIAL-3", Status: planner.StatusExcluded, Reason: "matched override"},
		{SerialNumber: "SERIAL-4", Status: planner.StatusUnmanaged, Reason: "no naming rule matched"},
		{SerialNumber: "SERIAL-5", Status: planner.StatusInvalid, Reason: "desired name exceeds length limit"},
		{SerialNumber: "SERIAL-6", Status: planner.StatusUnresolved, Reason: "variable resolved to conflicting values"},
	}
}

func TestJSONReportRetainsEveryDeviceAndPartialFailures(t *testing.T) {
	for _, all := range []bool{false, true} {
		results := outputFixture()
		results[0].Action, results[0].Error = app.ActionPending, "provider unavailable"
		var output bytes.Buffer
		if err := writeReport(&output, "json", all, true, results, errors.New("one rename failed")); err != nil {
			t.Fatal(err)
		}
		var report reconciliationReport
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Devices) != len(results) || report.Error != "one rename failed" {
			t.Fatalf("report = %#v", report)
		}
		device := report.Devices[0]
		if device.CurrentName != "OLD" || device.DesiredName != "NEW" || device.Action != app.ActionPending || device.Error != "provider unavailable" {
			t.Fatalf("device = %#v", device)
		}
	}
}

func TestHumanApplyReportsSubmittedAndPendingRenames(t *testing.T) {
	results := []app.Result{
		{SerialNumber: "FIRST", Status: planner.StatusRename, Action: app.ActionSubmitted},
		{SerialNumber: "SECOND", Status: planner.StatusRename, Action: app.ActionPending, Error: "try again"},
		{SerialNumber: "THIRD", Status: planner.StatusUnchanged, Error: "state unavailable"},
	}
	var output bytes.Buffer
	if err := writeReport(&output, "text", false, true, results, errors.New("try again")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"submitted", "pending", "try again", "THIRD", "state unavailable", "Renames: 1 submitted, 1 pending, 0 failed"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q: %s", want, output.String())
		}
	}
}
