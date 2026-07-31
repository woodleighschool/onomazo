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
		Item: planner.Item{
			Source:       "fixture",
			ID:           "device-1",
			SerialNumber: "SERIAL-1",
			Platform:     "ios",
			CurrentName:  "OLD-NAME",
			DesiredName:  "NEW-NAME",
			User:         "unit@example.invalid",
			Rule:         "fixture-rule",
			Status:       planner.StatusRename,
			Reason:       "name differs",
		},
		Action: app.ActionPlanned,
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
		"msg":      "device rename planned",
		"device":   "OLD-NAME",
		"platform": "ios",
		"serial":   "SERIAL-1",
		"user":     "unit@example.invalid",
		"to":       "NEW-NAME",
		"rule":     "fixture-rule",
		"status":   "rename",
		"action":   "planned",
	}
	for field, wantValue := range want {
		if got := record[field]; got != wantValue {
			t.Errorf("%s = %#v, want %#v", field, got, wantValue)
		}
	}
}

func TestWritePlanKeepsEmptyComparisonFields(t *testing.T) {
	t.Parallel()
	result := app.Result{Item: planner.Item{
		Source:       "fixture",
		ID:           "device-1",
		SerialNumber: "SERIAL-1",
		Platform:     "macos",
		CurrentName:  "FIXTURE",
		Status:       planner.StatusUnmanaged,
	}}
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

	var humanOutput bytes.Buffer
	if err := writePlan(&humanOutput, "human", []app.Result{result}); err != nil {
		t.Fatalf("writePlan(human) error = %v", err)
	}
	if !strings.Contains(humanOutput.String(), `serial="SERIAL-1"  user=""`) {
		t.Errorf("human plan = %q, want serial and user", humanOutput.String())
	}
}
