package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/app"
	"github.com/woodleighschool/onomazo/internal/planner"
)

func TestHumanSelectionKeepsCompleteMachineResults(t *testing.T) {
	for _, all := range []bool{false, true} {
		results := outputFixture()
		var human, machine bytes.Buffer
		if err := writeReport(&human, false, all, false, results, nil); err != nil {
			t.Fatal(err)
		}
		if err := writeReport(&machine, true, all, false, results, nil); err != nil {
			t.Fatal(err)
		}
		var report reconciliationReport
		decoder := json.NewDecoder(&machine)
		if err := decoder.Decode(&report); err != nil {
			t.Fatal(err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			t.Fatalf("trailing JSON: %v", err)
		}
		if report.Summary.Total != 6 || report.Summary.Statuses[planner.StatusUnchanged] != 1 {
			t.Fatalf("whole-run summary = %#v", report.Summary)
		}
		if !strings.Contains(human.String(), "6 total, 1 rename, 1 unchanged, 1 excluded, 1 unmanaged, 1 invalid, 1 unresolved") {
			t.Fatalf("human summary: %s", human.String())
		}
		for _, result := range results {
			want := all || result.Status != planner.StatusUnchanged
			if strings.Contains(human.String(), result.SerialNumber) != want {
				t.Fatalf("all=%t: human selection for %s: %s", all, result.Status, human.String())
			}
			found := false
			for _, selected := range report.Devices {
				found = found || selected.SerialNumber == result.SerialNumber
			}
			if !found {
				t.Fatalf("all=%t: JSON selection for %s = %t", all, result.Status, found)
			}
		}
	}
}

func TestApplyReportDistinguishesRequestOutcomes(t *testing.T) {
	retryAt := time.Date(2026, 9, 23, 4, 30, 0, 0, time.UTC)
	results := []app.Result{
		{ID: "submitted", Status: planner.StatusRename, CurrentName: "OLD-1", DesiredName: "NEW-1", Action: app.ActionSubmitted, Attempts: 1},
		{ID: "pending", Status: planner.StatusRename, CurrentName: "OLD-2", DesiredName: "NEW-2", Action: app.ActionPending, Attempts: 1},
		{ID: "retry", Status: planner.StatusRename, CurrentName: "OLD-3", DesiredName: "NEW-3", Action: app.ActionPending, Attempts: 2, RetryAt: retryAt, Error: "provider unavailable"},
		{ID: "failed", Status: planner.StatusRename, Action: app.ActionFailed, Error: "provider rejected request"},
		{ID: "not-submitted", Status: planner.StatusRename, Action: app.ActionPlanned, Error: "state unavailable"},
		{ID: "unchanged-error", Status: planner.StatusUnchanged, Error: "observe failed"},
	}
	var output bytes.Buffer
	if err := writeReport(&output, false, false, true, results, errors.New("rename errors")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`➤ "OLD-1" → "NEW-1": submitted`, `➤ "OLD-2" → "NEW-2": pending`, `➤ "OLD-3" → "NEW-3": retry`,
		"awaiting inventory confirmation", "Eligible after 2026-09-23T04:30:00Z on a subsequent apply/run cycle", "Attempts: 2",
		"failed", "not submitted", "unchanged-error", "observe failed", "provider unavailable", "provider rejected request",
		"Requests: 1 submitted, 2 pending, 1 failed, 1 not submitted", "Reconciliation incomplete",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q: %s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "renamed") || strings.Count(output.String(), "provider unavailable") != 1 {
		t.Fatalf("misleading or duplicate outcome: %s", output.String())
	}
	output.Reset()
	if err := writeReport(&output, true, false, true, results, errors.New("rename errors")); err != nil {
		t.Fatal(err)
	}
	var report reconciliationReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Error != "rename errors" || len(report.Devices) != len(results) || report.Devices[2].RetryAt != retryAt || report.Devices[2].Error != "provider unavailable" {
		t.Fatalf("partial JSON report = %#v", report)
	}
}

func TestHumanReportEscapesControlsAndPreservesValues(t *testing.T) {
	result := app.Result{Source: "source\x1b[31m", Namespace: "computers", ID: "device", CurrentName: "Café\nOLD", DesiredName: "NEW", User: "Zoë@example.invalid", Rule: "rule\tname", Status: planner.StatusRename, Action: app.ActionPlanned}
	var output bytes.Buffer
	if err := writeReport(&output, false, false, false, []app.Result{result}, nil); err != nil {
		t.Fatal(err)
	}
	for _, r := range output.String() {
		if r != '\n' && !strconv.IsPrint(r) {
			t.Fatalf("control rune %U in %q", r, output.String())
		}
	}
	for _, want := range []string{`"Café\nOLD" → "NEW"`, `Zoë@example.invalid`, `rule\tname`, `source\x1b[31m`, "planned", "not request eligibility"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
}

func TestEmptyAndUnavailableResults(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		var output bytes.Buffer
		if err := writeReport(&output, jsonOutput, false, false, nil, errors.New("inventory failed")); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 {
			t.Fatalf("fabricated report: %s", output.String())
		}
		if err := writeReport(&output, jsonOutput, false, false, []app.Result{}, nil); err != nil {
			t.Fatal(err)
		}
		if jsonOutput {
			if !strings.Contains(output.String(), `"devices":[]`) || !strings.Contains(output.String(), `"total":0`) {
				t.Fatalf("empty inventory: %s", output.String())
			}
		} else if !strings.Contains(output.String(), "0 total") {
			t.Fatalf("empty inventory: %s", output.String())
		}
	}
}

func outputFixture() []app.Result {
	return []app.Result{
		{SerialNumber: "SERIAL-1", Status: planner.StatusRename, CurrentName: "OLD", DesiredName: "NEW", Reason: "name differs", Action: app.ActionPlanned},
		{SerialNumber: "SERIAL-2", Status: planner.StatusUnchanged, CurrentName: "CURRENT", DesiredName: "CURRENT", Reason: "name already matches"},
		{SerialNumber: "SERIAL-3", Status: planner.StatusExcluded, Rule: "exclusion", Reason: "matched override"},
		{SerialNumber: "SERIAL-4", Status: planner.StatusUnmanaged, Reason: "no naming rule matched"},
		{SerialNumber: "SERIAL-5", Status: planner.StatusInvalid, Reason: "desired name exceeds length limit"},
		{SerialNumber: "SERIAL-6", Status: planner.StatusUnresolved, Reason: "variable resolved to conflicting values"},
	}
}
