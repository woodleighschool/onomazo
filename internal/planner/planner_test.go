package planner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/domain"
)

func TestPlanEvaluatesCompletePolicy(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, `
  constraints:
    max_length: 15
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  overrides:
    - name: excluded-serial
      when: 'device.serial_number == "EXCLUDED"'
      exclude: true
    - name: fixed-library
      when: 'device.serial_number == "LIBRARY"'
      desired_name: LIBRARY-01
  variables:
    role:
      cases:
        - when: '"students" in user.groups'
          value: STU
        - when: '"staff" in user.groups'
          value: EMP
  rules:
    - name: assigned-user
      when: 'user.present && "role" in vars && user.mail_nickname != ""'
      desired_name: '(vars["role"] + "-" + slug(user.mail_nickname)).upperAscii()'
  collisions:
    rank:
      - expression: device.enrolled_at
        order: ascending
    disambiguate:
      type: sequence
      separator: '-'
      preserve_existing: true
`)

	plan, err := planner.Plan([]Record{
		record("unmanaged", "NONE", "OLD", domain.User{}),
		record("student", "STUDENT", "OLD", user("alex", "students")),
		record("overlap", "OVERLAP", "OLD", user("lee", "students", "staff")),
		record("excluded", "EXCLUDED", "OLD", user("casey", "staff")),
		record("library", "LIBRARY", "OLD", domain.User{}),
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	want := []Item{
		{Source: "intune", ID: "excluded", SerialNumber: "EXCLUDED", Platform: "macos", CurrentName: "OLD", User: "casey@example.com", Rule: "excluded-serial", Status: StatusExcluded, Reason: "matched override"},
		{Source: "intune", ID: "library", SerialNumber: "LIBRARY", Platform: "macos", CurrentName: "OLD", DesiredName: "LIBRARY-01", Rule: "fixed-library", Status: StatusRename, Reason: "name differs"},
		{Source: "intune", ID: "overlap", SerialNumber: "OVERLAP", Platform: "macos", CurrentName: "OLD", User: "lee@example.com", Status: StatusUnresolved, Reason: `variable "role" resolved to conflicting values`},
		{Source: "intune", ID: "student", SerialNumber: "STUDENT", Platform: "macos", CurrentName: "OLD", DesiredName: "STU-ALEX", User: "alex@example.com", Rule: "assigned-user", Status: StatusRename, Reason: "name differs"},
		{Source: "intune", ID: "unmanaged", SerialNumber: "NONE", Platform: "macos", CurrentName: "OLD", Status: StatusUnmanaged, Reason: "no naming rule matched"},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("Plan() = %#v, want %#v", plan, want)
	}
}

func TestPlanIsDeterministicAndPreservesExistingName(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, basicNaming(15, true))
	first := record("first", "FIRST", "OLD", user("alex", "staff"))
	first.Device.EnrolledAt = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	second := record("second", "SECOND", "EMP-ALEX", user("alex", "staff"))
	second.Device.EnrolledAt = time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

	forward, err := planner.Plan([]Record{first, second})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	reverse, err := planner.Plan([]Record{second, first})
	if err != nil {
		t.Fatalf("Plan() reversed error = %v", err)
	}
	if !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("Plan() depends on input order: %#v != %#v", forward, reverse)
	}

	if got, want := forward[0].DesiredName, "EMP-ALEX-2"; got != want {
		t.Errorf("first desired name = %q, want %q", got, want)
	}
	if got, want := forward[1].Status, StatusUnchanged; got != want {
		t.Errorf("second status = %q, want %q", got, want)
	}
}

func TestPlanReservesUnmanagedCurrentNames(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, basicNaming(15, false))
	plan, err := planner.Plan([]Record{
		record("reserved", "RESERVED", "EMP-ALEX", domain.User{}),
		record("managed", "MANAGED", "OLD", user("alex", "staff")),
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got, want := plan[0].DesiredName, "EMP-ALEX-2"; got != want {
		t.Errorf("managed desired name = %q, want %q", got, want)
	}
}

func TestPlanRejectsAuthoritativeCollision(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, `
  constraints:
    max_length: 15
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  overrides:
    - name: fixed
      when: 'device.serial_number.startsWith("FIXED")'
      desired_name: FIXED-01
  rules:
    - name: fallback
      when: 'true'
      desired_name: '"FALLBACK"'
  collisions:
    disambiguate:
      type: sequence
      separator: '-'
      preserve_existing: true
`)
	plan, err := planner.Plan([]Record{
		record("one", "FIXED-ONE", "OLD-1", domain.User{}),
		record("two", "FIXED-TWO", "OLD-2", domain.User{}),
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	for _, item := range plan {
		if item.Status != StatusInvalid {
			t.Errorf("%s status = %q, want %q", item.ID, item.Status, StatusInvalid)
		}
		if !strings.Contains(item.Reason, "authoritative name conflicts") {
			t.Errorf("%s reason = %q, want authoritative conflict", item.ID, item.Reason)
		}
	}
}

func TestPlanNeverTruncatesCollisionSuffix(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, basicNaming(8, false))
	plan, err := planner.Plan([]Record{
		record("one", "ONE", "OLD-1", user("alex", "staff")),
		record("two", "TWO", "OLD-2", user("alex", "staff")),
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got, want := plan[0].DesiredName, "EMP-ALEX"; got != want {
		t.Errorf("winner desired name = %q, want %q", got, want)
	}
	if got, want := plan[1].Status, StatusInvalid; got != want {
		t.Errorf("loser status = %q, want %q", got, want)
	}
	if !strings.Contains(plan[1].Reason, "suffix exceeds") {
		t.Errorf("loser reason = %q, want suffix length error", plan[1].Reason)
	}
}

func TestPlanAppliesWindowsProviderLimit(t *testing.T) {
	t.Parallel()

	planner := loadPlanner(t, `
  constraints:
    max_length: 63
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  rules:
    - name: long-name
      when: 'true'
      desired_name: '"SIXTEEN-CHAR-NAME"'
  collisions:
    disambiguate:
      type: sequence
      separator: '-'
      preserve_existing: true
`)
	item := record("windows", "WINDOWS", "OLD", domain.User{})
	item.Device.Platform = "windows"
	plan, err := planner.Plan([]Record{item})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got, want := plan[0].Status, StatusInvalid; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if !strings.Contains(plan[0].Reason, "maximum length 15") {
		t.Errorf("reason = %q, want Windows length error", plan[0].Reason)
	}
}

func loadPlanner(t *testing.T, naming string) *Planner {
	t.Helper()

	contents := fmt.Sprintf(`version: 1
connections:
  microsoft:
    type: microsoft_graph
    tenant_id: tenant
    client_id: client
    client_secret: secret
devices:
  - name: intune
    type: intune
    connection: microsoft
    platforms: [macos, ios, windows]
naming:
%s`, naming)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	planner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return planner
}

func basicNaming(maximumLength int, preserveExisting bool) string {
	return fmt.Sprintf(`
  constraints:
    max_length: %d
    pattern: '^[A-Z0-9][A-Z0-9-]*$'
  variables:
    role:
      cases:
        - when: '"staff" in user.groups'
          value: EMP
  rules:
    - name: assigned-user
      when: 'user.present && "role" in vars'
      desired_name: '(vars["role"] + "-" + slug(user.mail_nickname)).upperAscii()'
  collisions:
    rank:
      - expression: device.enrolled_at
        order: ascending
    disambiguate:
      type: sequence
      separator: '-'
      preserve_existing: %t
`, maximumLength, preserveExisting)
}

func record(id, serial, currentName string, primaryUser domain.User) Record {
	return Record{
		Device: domain.Device{
			Source:       "intune",
			ID:           id,
			SerialNumber: serial,
			CurrentName:  currentName,
			Platform:     "macos",
			EnrolledAt:   time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		User: primaryUser,
	}
}

func user(mailNickname string, groups ...string) domain.User {
	return domain.User{
		Present:           true,
		MailNickname:      mailNickname,
		UserPrincipalName: mailNickname + "@example.com",
		Groups:            groups,
	}
}
