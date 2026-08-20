package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

func TestWriteJSONPlanKeepsBaselineComparisonFields(t *testing.T) {
	t.Parallel()
	result := app.Result{
		Source:       "fixture",
		Namespace:    "devices",
		ID:           "device-1",
		SerialNumber: "SERIAL-1",
		Platform:     "ios",
		CurrentName:  "OLD-NAME",
		DesiredName:  "NEW-NAME",
		User:         "unit@example.invalid",
		Rule:         "fixture-rule",
		Status:       planner.StatusRename,
		Reason:       "name differs",
		Action:       app.ActionPlanned,
	}
	var output bytes.Buffer
	if err := writePlan(&output, "json", []app.Result{result}); err != nil {
		t.Fatalf("writePlan() error = %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	want := map[string]string{
		"msg":       "device rename planned",
		"namespace": "devices",
		"device":    "OLD-NAME",
		"platform":  "ios",
		"serial":    "SERIAL-1",
		"user":      "unit@example.invalid",
		"to":        "NEW-NAME",
		"rule":      "fixture-rule",
		"status":    "rename",
		"action":    "planned",
	}
	for field, wantValue := range want {
		if got := record[field]; got != wantValue {
			t.Errorf("%s = %#v, want %#v", field, got, wantValue)
		}
	}
}

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
	if err := writePlan(&output, "human", results); err != nil {
		t.Fatalf("writePlan() error = %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if got, want := len(lines), 3; got != want {
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

func TestWriteJSONPlanKeepsEmptyComparisonFields(t *testing.T) {
	t.Parallel()
	result := app.Result{
		Source:       "fixture",
		ID:           "device-1",
		SerialNumber: "SERIAL-1",
		Platform:     "macos",
		CurrentName:  "FIXTURE",
		Status:       planner.StatusUnmanaged}
	var jsonOutput bytes.Buffer
	if err := writePlan(&jsonOutput, "json", []app.Result{result}); err != nil {
		t.Fatalf("writePlan(json) error = %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(jsonOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	for _, field := range []string{"user", "to", "rule", "action"} {
		if value, exists := record[field]; !exists || value != "" {
			t.Errorf("%s = %#v, exists %t, want empty field", field, value, exists)
		}
	}
}
