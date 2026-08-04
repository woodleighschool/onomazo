package state

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestLedgerDoesNotRetrySubmittedRename(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ledger, err := Open(store)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := Key{Source: "intune", Namespace: "managed_devices", DeviceID: "device-1"}
	started := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	cooldown := 30 * time.Minute

	decision, err := ledger.Prepare(key, "SERIAL", "OLD", "NEW", started, cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 1)
	assertStoredAttempt(t, store, 1, started)
	if err := ledger.MarkSubmitted(key, "SERIAL", "NEW"); err != nil {
		t.Fatalf("MarkSubmitted() error = %v", err)
	}

	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "NEW", started.Add(time.Minute), cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() during cooldown error = %v", err)
	}
	assertDecision(t, decision, DispositionPending, 1)
	assertStoredAttempt(t, store, 1, started)

	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "NEW", started.Add(24*time.Hour), cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() after cooldown error = %v", err)
	}
	assertDecision(t, decision, DispositionPending, 1)
	assertStoredAttempt(t, store, 1, started)
	intents, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := intents[0].Status, IntentSubmitted; got != want {
		t.Errorf("stored status = %q, want %q", got, want)
	}
}

func TestLedgerRetriesFailedSubmissionAndStopsAfterLimit(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ledger, err := Open(store)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := Key{Source: "intune", Namespace: "managed_devices", DeviceID: "device-1"}
	started := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	cooldown := 30 * time.Minute

	decision, err := ledger.Prepare(key, "SERIAL", "OLD", "NEW", started, cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 1)
	if err := ledger.MarkRetrying(key, "SERIAL", "NEW", "provider unavailable"); err != nil {
		t.Fatalf("MarkRetrying() error = %v", err)
	}

	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "NEW", started.Add(time.Minute), cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() during cooldown error = %v", err)
	}
	assertDecision(t, decision, DispositionPending, 1)

	retriedAt := started.Add(cooldown)
	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "NEW", retriedAt, cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() retry error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 2)
	if err := ledger.MarkRetrying(key, "SERIAL", "NEW", "provider still unavailable"); err != nil {
		t.Fatalf("MarkRetrying() second error = %v", err)
	}

	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "NEW", retriedAt.Add(cooldown), cooldown, 2)
	if err != nil {
		t.Fatalf("Prepare() exhausted error = %v", err)
	}
	assertDecision(t, decision, DispositionFailed, 2)
	if got, want := decision.Failure, "provider still unavailable"; got != want {
		t.Errorf("failure = %q, want %q", got, want)
	}
	intents := ledger.Snapshot()
	if got, want := intents[0].Status, IntentFailed; got != want {
		t.Errorf("stored status = %q, want %q", got, want)
	}
}

func TestLedgerSupersedesChangedDesiredName(t *testing.T) {
	t.Parallel()

	ledger, err := Open(NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := Key{Source: "jamf", Namespace: "computers", DeviceID: "42"}
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	if _, err := ledger.Prepare(key, "SERIAL", "OLD", "FIRST", now, time.Hour, 3); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	decision, err := ledger.Prepare(key, "SERIAL", "OLD", "SECOND", now.Add(time.Minute), time.Hour, 3)
	if err != nil {
		t.Fatalf("Prepare() changed desired name error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 1)
	intent := ledger.Snapshot()[0]
	if got, want := intent.DesiredName, "SECOND"; got != want {
		t.Errorf("desired name = %q, want %q", got, want)
	}
}

func TestLedgerSeparatesProviderNamespaces(t *testing.T) {
	t.Parallel()

	ledger, err := Open(NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	keys := []Key{
		{Source: "jamf", Namespace: "computers", DeviceID: "shared-id"},
		{Source: "jamf", Namespace: "mobile_devices", DeviceID: "shared-id"},
	}
	for index, key := range keys {
		if _, err := ledger.Prepare(
			key,
			"SERIAL-"+key.Namespace,
			"OLD",
			"NEW",
			now.Add(time.Duration(index)*time.Minute),
			time.Hour,
			3,
		); err != nil {
			t.Fatalf("Prepare(%s) error = %v", key.Namespace, err)
		}
	}

	intents := ledger.Snapshot()
	if got, want := len(intents), 2; got != want {
		t.Fatalf("intents = %d, want %d", got, want)
	}
	if got, want := []string{intents[0].Namespace, intents[1].Namespace}, []string{"computers", "mobile_devices"}; !reflect.DeepEqual(got, want) {
		t.Errorf("intent namespaces = %#v, want %#v", got, want)
	}
}

func TestLedgerClearsObservedAndReusedDevices(t *testing.T) {
	t.Parallel()

	ledger, err := Open(NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := Key{Source: "intune", Namespace: "managed_devices", DeviceID: "device"}
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	if _, err := ledger.Prepare(key, "SERIAL-1", "OLD", "NEW", now, time.Hour, 3); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	cleared, err := ledger.Observe(key, "SERIAL-1", "NEW")
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if !cleared || len(ledger.Snapshot()) != 0 {
		t.Errorf("Observe() cleared = %t, snapshot = %#v", cleared, ledger.Snapshot())
	}

	if _, err := ledger.Prepare(key, "SERIAL-1", "OLD", "NEW", now, time.Hour, 3); err != nil {
		t.Fatalf("Prepare() second error = %v", err)
	}
	decision, err := ledger.Prepare(key, "SERIAL-2", "OLD", "NEW", now.Add(time.Minute), time.Hour, 3)
	if err != nil {
		t.Fatalf("Prepare() reused device error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 1)
	if got, want := ledger.Snapshot()[0].SerialNumber, "SERIAL-2"; got != want {
		t.Errorf("serial number = %q, want %q", got, want)
	}
}

func TestLedgerKeepsExplicitFailureUntilDesiredNameChanges(t *testing.T) {
	t.Parallel()

	ledger, err := Open(NewMemoryStore())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	key := Key{Source: "intune", Namespace: "managed_devices", DeviceID: "device"}
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	if _, err := ledger.Prepare(key, "SERIAL", "OLD", "NEW", now, time.Hour, 3); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := ledger.MarkFailed(key, "SERIAL", "NEW", "provider rejected rename"); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	decision, err := ledger.Prepare(key, "SERIAL", "OLD", "NEW", now.Add(2*time.Hour), time.Hour, 3)
	if err != nil {
		t.Fatalf("Prepare() failed intent error = %v", err)
	}
	assertDecision(t, decision, DispositionFailed, 1)
	if got, want := decision.Failure, "provider rejected rename"; got != want {
		t.Errorf("failure = %q, want %q", got, want)
	}

	decision, err = ledger.Prepare(key, "SERIAL", "OLD", "BETTER", now.Add(2*time.Hour), time.Hour, 3)
	if err != nil {
		t.Fatalf("Prepare() replacement error = %v", err)
	}
	assertDecision(t, decision, DispositionSubmit, 1)
}

func TestLedgerRollsBackWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	store := &failingStore{saveError: errors.New("disk full")}
	ledger, err := Open(store)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	_, err = ledger.Prepare(
		Key{Source: "intune", Namespace: "managed_devices", DeviceID: "device"},
		"SERIAL",
		"OLD",
		"NEW",
		time.Now(),
		time.Hour,
		3,
	)
	if err == nil {
		t.Fatal("Prepare() error = nil, want persistence error")
	}
	if got := ledger.Snapshot(); len(got) != 0 {
		t.Errorf("Snapshot() = %#v, want empty rollback", got)
	}
}

func assertDecision(t *testing.T, decision Decision, disposition Disposition, attempts int) {
	t.Helper()
	if got, want := decision.Disposition, disposition; got != want {
		t.Errorf("disposition = %q, want %q", got, want)
	}
	if got, want := decision.Attempts, attempts; got != want {
		t.Errorf("attempts = %d, want %d", got, want)
	}
}

func assertStoredAttempt(t *testing.T, store Store, attempts int, attemptedAt time.Time) {
	t.Helper()
	intents, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(intents), 1; got != want {
		t.Fatalf("stored intents = %d, want %d", got, want)
	}
	if got, want := intents[0].Attempts, attempts; got != want {
		t.Errorf("stored attempts = %d, want %d", got, want)
	}
	if got, want := intents[0].AttemptedAt, attemptedAt; !got.Equal(want) {
		t.Errorf("attempted at = %v, want %v", got, want)
	}
}

type failingStore struct {
	intents   []Intent
	saveError error
}

func (s *failingStore) Load() ([]Intent, error) {
	return append([]Intent(nil), s.intents...), nil
}

func (s *failingStore) Save(intents []Intent) error {
	if s.saveError != nil {
		return s.saveError
	}
	s.intents = append([]Intent(nil), intents...)
	return nil
}

var _ Store = (*failingStore)(nil)
