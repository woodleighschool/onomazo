package app

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/domain"
	"github.com/woodleighschool/onomazo/internal/planner"
	"github.com/woodleighschool/onomazo/internal/state"
)

func TestReconcilePlanIsReadOnly(t *testing.T) {
	t.Parallel()
	source := &fakeSource{name: "fixture", devices: []domain.Device{{
		ID:           "device-1",
		SerialNumber: "SERIAL-1",
		CurrentName:  "OLD",
		Platform:     "macos",
		UserID:       "unit",
	}}}
	intentLedger := &spyLedger{}
	service := newTestService(t, Options{
		Sources:  []DeviceSource{source},
		Identity: &fakeIdentity{},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			return []planner.Item{renameItem(records[0], "NEW")}, nil
		}},
		Ledger: intentLedger,
	})

	results, err := service.Reconcile(context.Background(), false)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := results[0].Action, ActionPlanned; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
	if source.renameCount != 0 {
		t.Errorf("renames = %d, want 0", source.renameCount)
	}
	if intentLedger.prepareCount != 0 || intentLedger.observeCount != 0 {
		t.Errorf("plan mutated ledger: %#v", intentLedger)
	}
}

func TestReconcileRefreshesPrimaryUserImmediatelyAndStableDetailsOnTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	source := &fakeSource{name: "fixture"}
	identity := &fakeIdentity{}
	var planned [][]planner.Record
	service := newTestService(t, Options{
		Sources:  []DeviceSource{source},
		Identity: identity,
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			planned = append(planned, append([]planner.Record(nil), records...))
			return []planner.Item{unmanagedItem(records[0])}, nil
		}},
		Ledger:           &spyLedger{},
		DeviceDetailsTTL: 10 * time.Minute,
		IdentityTTL:      time.Hour,
		Now:              func() time.Time { return now },
	})

	source.devices = []domain.Device{fixtureDevice("FIRST", "MODEL-A", "user-1")}
	mustReconcile(t, service, false)
	now = now.Add(time.Minute)
	source.devices = []domain.Device{fixtureDevice("SECOND", "MODEL-B", "user-1")}
	mustReconcile(t, service, false)
	now = now.Add(time.Minute)
	source.devices = []domain.Device{fixtureDevice("THIRD", "MODEL-C", "user-2")}
	mustReconcile(t, service, false)
	now = now.Add(9 * time.Minute)
	source.devices = []domain.Device{fixtureDevice("FOURTH", "MODEL-D", "user-2")}
	mustReconcile(t, service, false)

	if got, want := len(identity.calls), 2; got != want {
		t.Fatalf("identity calls = %d, want %d", got, want)
	}
	if got, want := identity.calls, [][]string{{"user-1"}, {"user-2"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("identity identifiers = %#v, want %#v", got, want)
	}
	if got, want := planned[1][0].Device.CurrentName, "SECOND"; got != want {
		t.Errorf("current name before details TTL = %q, want %q", got, want)
	}
	if got, want := planned[1][0].Device.Model, "MODEL-A"; got != want {
		t.Errorf("model before details TTL = %q, want %q", got, want)
	}
	if got, want := planned[2][0].Device.Model, "MODEL-C"; got != want {
		t.Errorf("model after primary user change = %q, want %q", got, want)
	}
	if got, want := planned[3][0].Device.Model, "MODEL-C"; got != want {
		t.Errorf("model before refreshed TTL = %q, want %q", got, want)
	}

	now = now.Add(time.Minute)
	source.devices = []domain.Device{fixtureDevice("FIFTH", "MODEL-E", "user-2")}
	mustReconcile(t, service, false)
	if got, want := planned[4][0].Device.Model, "MODEL-E"; got != want {
		t.Errorf("model at details TTL = %q, want %q", got, want)
	}
}

func TestReconcileCooldownPreventsRepeatedRenameSubmissions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	desiredName := "NEW"
	source := &fakeSource{name: "fixture", devices: []domain.Device{fixtureDevice("OLD", "MODEL", "unit")}}
	store := state.NewMemoryStore()
	intentLedger, err := state.Open(store)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	service := newTestService(t, Options{
		Sources: []DeviceSource{source},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			if records[0].Device.CurrentName == desiredName {
				return []planner.Item{unchangedItem(records[0], desiredName)}, nil
			}
			return []planner.Item{renameItem(records[0], desiredName)}, nil
		}},
		Ledger:           intentLedger,
		RenameRetryAfter: 10 * time.Minute,
		Now:              func() time.Time { return now },
	})

	first := mustReconcile(t, service, true)
	if got, want := first[0].Action, ActionSubmitted; got != want {
		t.Fatalf("first action = %q, want %q", got, want)
	}
	now = now.Add(time.Minute)
	second := mustReconcile(t, service, true)
	if got, want := second[0].Action, ActionPending; got != want {
		t.Fatalf("second action = %q, want %q", got, want)
	}
	if got, want := source.renameCount, 1; got != want {
		t.Fatalf("renames during cooldown = %d, want %d", got, want)
	}

	now = now.Add(9 * time.Minute)
	third := mustReconcile(t, service, true)
	if got, want := third[0].Action, ActionSubmitted; got != want {
		t.Fatalf("action at retry time = %q, want %q", got, want)
	}
	desiredName = "OTHER"
	now = now.Add(time.Minute)
	fourth := mustReconcile(t, service, true)
	if got, want := fourth[0].Action, ActionSubmitted; got != want {
		t.Fatalf("changed desired action = %q, want %q", got, want)
	}
	if got, want := source.renameCount, 3; got != want {
		t.Fatalf("total renames = %d, want %d", got, want)
	}

	source.devices[0].CurrentName = "OTHER"
	now = now.Add(time.Minute)
	mustReconcile(t, service, true)
	if intents := intentLedger.Snapshot(); len(intents) != 0 {
		t.Errorf("observed rename intents = %#v, want none", intents)
	}
}

func TestReconcileInvalidPlanPreservesPendingCooldown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	status := planner.StatusRename
	source := &fakeSource{name: "fixture", devices: []domain.Device{fixtureDevice("OLD", "MODEL", "unit")}}
	intentLedger, err := state.Open(state.NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	service := newTestService(t, Options{
		Sources: []DeviceSource{source},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			item := renameItem(records[0], "NEW")
			item.Status = status
			return []planner.Item{item}, nil
		}},
		Ledger:           intentLedger,
		RenameRetryAfter: 10 * time.Minute,
		Now:              func() time.Time { return now },
	})

	mustReconcile(t, service, true)
	status = planner.StatusInvalid
	now = now.Add(time.Minute)
	mustReconcile(t, service, true)
	status = planner.StatusRename
	now = now.Add(time.Minute)
	results := mustReconcile(t, service, true)
	if got, want := results[0].Action, ActionPending; got != want {
		t.Errorf("action after transient invalid plan = %q, want %q", got, want)
	}
	if got, want := source.renameCount, 1; got != want {
		t.Errorf("renames after transient invalid plan = %d, want %d", got, want)
	}
}

func TestReconcileMarksPermanentProviderRejectionFailed(t *testing.T) {
	t.Parallel()
	source := &fakeSource{
		name:      "fixture",
		devices:   []domain.Device{fixtureDevice("OLD", "MODEL", "unit")},
		renameErr: statusError{status: http.StatusBadRequest},
	}
	intentLedger, err := state.Open(state.NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	service := newTestService(t, Options{
		Sources: []DeviceSource{source},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			return []planner.Item{renameItem(records[0], "NEW")}, nil
		}},
		Ledger: intentLedger,
	})

	results, reconcileErr := service.Reconcile(context.Background(), true)
	if reconcileErr == nil {
		t.Fatal("Reconcile() error = nil, want rename error")
	}
	if got, want := results[0].Action, ActionFailed; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
	results, _ = service.Reconcile(context.Background(), true)
	if got, want := source.renameCount, 1; got != want {
		t.Errorf("renames after permanent failure = %d, want %d", got, want)
	}
	if got, want := results[0].Action, ActionFailed; got != want {
		t.Errorf("subsequent action = %q, want %q", got, want)
	}
}

func TestReconcileRetriesAuthorizationFailureAfterCooldown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	source := &fakeSource{
		name:      "fixture",
		devices:   []domain.Device{fixtureDevice("OLD", "MODEL", "unit")},
		renameErr: statusError{status: http.StatusForbidden},
	}
	intentLedger, err := state.Open(state.NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	service := newTestService(t, Options{
		Sources: []DeviceSource{source},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			return []planner.Item{renameItem(records[0], "NEW")}, nil
		}},
		Ledger:           intentLedger,
		RenameRetryAfter: 10 * time.Minute,
		Now:              func() time.Time { return now },
	})

	results, reconcileErr := service.Reconcile(context.Background(), true)
	if reconcileErr == nil {
		t.Fatal("Reconcile() error = nil, want authorization error")
	}
	if got, want := results[0].Action, ActionPending; got != want {
		t.Errorf("authorization action = %q, want %q", got, want)
	}
	source.renameErr = nil
	now = now.Add(10 * time.Minute)
	results = mustReconcile(t, service, true)
	if got, want := results[0].Action, ActionSubmitted; got != want {
		t.Errorf("recovered action = %q, want %q", got, want)
	}
	if got, want := source.renameCount, 2; got != want {
		t.Errorf("renames after recovery = %d, want %d", got, want)
	}
}

func TestReconcileStopsPreparingIntentsAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	intentLedger := &cancelingLedger{cancel: cancel}
	source := &fakeSource{name: "fixture", devices: []domain.Device{
		{ID: "device-1", SerialNumber: "SERIAL-1", CurrentName: "OLD-1", Platform: "macos"},
		{ID: "device-2", SerialNumber: "SERIAL-2", CurrentName: "OLD-2", Platform: "macos"},
		{ID: "device-3", SerialNumber: "SERIAL-3", CurrentName: "OLD-3", Platform: "macos"},
	}}
	service := newTestService(t, Options{
		Sources: []DeviceSource{source},
		Planner: fakePlanner{plan: func(records []planner.Record) ([]planner.Item, error) {
			items := make([]planner.Item, len(records))
			for index, record := range records {
				items[index] = renameItem(record, "NEW-"+record.Device.ID)
			}
			return items, nil
		}},
		Ledger: intentLedger,
	})

	_, err := service.Reconcile(ctx, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Reconcile() error = %v, want context canceled", err)
	}
	if got, want := intentLedger.prepareCount, 1; got != want {
		t.Errorf("prepared intents = %d, want %d", got, want)
	}
	if got := source.renameCount; got != 0 {
		t.Errorf("renames after cancellation = %d, want 0", got)
	}
}

type fakeSource struct {
	mutex       sync.Mutex
	name        string
	devices     []domain.Device
	renameErr   error
	renameCount int
}

func (s *fakeSource) Name() string {
	return s.name
}

func (s *fakeSource) ListDevices(context.Context) ([]domain.Device, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]domain.Device(nil), s.devices...), nil
}

func (s *fakeSource) Rename(_ context.Context, _ domain.Device, _ string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.renameCount++
	return s.renameErr
}

func (*fakeSource) StatusCode(err error) (int, bool) {
	var statusErr statusError
	if !errors.As(err, &statusErr) {
		return 0, false
	}
	return statusErr.status, true
}

type statusError struct {
	status int
}

func (e statusError) Error() string {
	return http.StatusText(e.status)
}

type fakeIdentity struct {
	mutex sync.Mutex
	calls [][]string
}

func (r *fakeIdentity) ResolveUsers(
	_ context.Context,
	identifiers []string,
	_ map[string][]string,
	_ int,
) (map[string]domain.User, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.calls = append(r.calls, append([]string(nil), identifiers...))
	users := make(map[string]domain.User, len(identifiers))
	for _, identifier := range identifiers {
		users[identifier] = domain.User{
			Present:           true,
			ID:                "object-" + identifier,
			MailNickname:      identifier,
			UserPrincipalName: identifier + "@example.invalid",
		}
	}
	return users, nil
}

type fakePlanner struct {
	plan func([]planner.Record) ([]planner.Item, error)
}

func (p fakePlanner) Plan(records []planner.Record) ([]planner.Item, error) {
	return p.plan(records)
}

type spyLedger struct {
	prepareCount int
	observeCount int
}

type cancelingLedger struct {
	spyLedger
	cancel context.CancelFunc
}

func (l *cancelingLedger) Prepare(
	state.Key,
	string,
	string,
	string,
	time.Time,
	time.Duration,
	int,
) (state.Decision, error) {
	l.prepareCount++
	l.cancel()
	return state.Decision{Disposition: state.DispositionSubmit, Attempts: 1}, nil
}

func (l *spyLedger) Prepare(
	state.Key,
	string,
	string,
	string,
	time.Time,
	time.Duration,
	int,
) (state.Decision, error) {
	l.prepareCount++
	return state.Decision{Disposition: state.DispositionSubmit, Attempts: 1}, nil
}

func (*spyLedger) MarkFailed(state.Key, string, string, string) error {
	return nil
}

func (l *spyLedger) Observe(state.Key, string, string) (bool, error) {
	l.observeCount++
	return false, nil
}

func newTestService(t *testing.T, options Options) *Service {
	t.Helper()
	if options.DeviceDetailsTTL == 0 {
		options.DeviceDetailsTTL = time.Hour
	}
	if options.IdentityTTL == 0 {
		options.IdentityTTL = time.Hour
	}
	if options.RenameRetryAfter == 0 {
		options.RenameRetryAfter = time.Hour
	}
	if options.RenameMaxAttempts == 0 {
		options.RenameMaxAttempts = 3
	}
	if options.Concurrency == 0 {
		options.Concurrency = 2
	}
	service, err := New(options)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func fixtureDevice(name, model, userID string) domain.Device {
	return domain.Device{
		ID:           "device-1",
		SerialNumber: "SERIAL-1",
		CurrentName:  name,
		Platform:     "macos",
		Model:        model,
		UserID:       userID,
	}
}

func renameItem(record planner.Record, desiredName string) planner.Item {
	return planner.Item{
		Source:       record.Device.Source,
		ID:           record.Device.ID,
		SerialNumber: record.Device.SerialNumber,
		Platform:     record.Device.Platform,
		CurrentName:  record.Device.CurrentName,
		DesiredName:  desiredName,
		User:         record.User.UserPrincipalName,
		Rule:         "fixture-rule",
		Status:       planner.StatusRename,
		Reason:       "name differs",
	}
}

func unchangedItem(record planner.Record, desiredName string) planner.Item {
	item := renameItem(record, desiredName)
	item.Status = planner.StatusUnchanged
	item.Reason = "name already matches"
	return item
}

func unmanagedItem(record planner.Record) planner.Item {
	return planner.Item{
		Source:       record.Device.Source,
		ID:           record.Device.ID,
		SerialNumber: record.Device.SerialNumber,
		Platform:     record.Device.Platform,
		CurrentName:  record.Device.CurrentName,
		User:         record.User.UserPrincipalName,
		Status:       planner.StatusUnmanaged,
	}
}

func mustReconcile(t *testing.T, service *Service, apply bool) []Result {
	t.Helper()
	results, err := service.Reconcile(context.Background(), apply)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	return results
}
