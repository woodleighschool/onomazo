package state

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Ledger applies cooldown and retry policy to persisted rename intentions.
type Ledger struct {
	mutex   sync.Mutex
	store   Store
	intents map[Key]Intent
}

// Open loads and validates the configured intent store.
func Open(store Store) (*Ledger, error) {
	if store == nil {
		return nil, fmt.Errorf("state store is required")
	}
	loaded, err := store.Load()
	if err != nil {
		return nil, err
	}
	intents := make(map[Key]Intent, len(loaded))
	for index, intent := range loaded {
		if err := validateIntent(intent); err != nil {
			return nil, fmt.Errorf("state intent %d: %w", index, err)
		}
		key := Key{Source: intent.Source, Namespace: intent.Namespace, DeviceID: intent.DeviceID}
		if _, exists := intents[key]; exists {
			return nil, fmt.Errorf(
				"state contains duplicate device %s/%s/%s",
				key.Source,
				key.Namespace,
				key.DeviceID,
			)
		}
		intents[key] = intent
	}
	return &Ledger{store: store, intents: intents}, nil
}

// Prepare persists a rename attempt before telling the caller to submit it.
func (l *Ledger) Prepare(
	key Key,
	serialNumber string,
	currentName string,
	desiredName string,
	now time.Time,
	retryAfter time.Duration,
	maxAttempts int,
) (Decision, error) {
	if err := validateRequest(key, serialNumber, desiredName, retryAfter, maxAttempts); err != nil {
		return Decision{}, err
	}
	now = now.UTC()

	l.mutex.Lock()
	defer l.mutex.Unlock()
	existing, found := l.intents[key]
	if found && existing.SerialNumber != serialNumber {
		if err := l.deleteLocked(key); err != nil {
			return Decision{}, err
		}
		found = false
	}
	if currentName == desiredName {
		if found {
			if err := l.deleteLocked(key); err != nil {
				return Decision{}, err
			}
		}
		return Decision{Disposition: DispositionObserved}, nil
	}
	if !found || existing.DesiredName != desiredName {
		intent := Intent{
			Source:       key.Source,
			Namespace:    key.Namespace,
			DeviceID:     key.DeviceID,
			SerialNumber: serialNumber,
			DesiredName:  desiredName,
			AttemptedAt:  now,
			Attempts:     1,
			Status:       IntentRetrying,
		}
		if err := l.putLocked(key, intent); err != nil {
			return Decision{}, err
		}
		return Decision{Disposition: DispositionSubmit, Attempts: 1, RetryAt: now.Add(retryAfter)}, nil
	}

	switch existing.Status {
	case IntentSubmitted:
		return Decision{Disposition: DispositionPending, Attempts: existing.Attempts}, nil
	case IntentRetrying:
	case IntentFailed:
		return Decision{Disposition: DispositionFailed, Attempts: existing.Attempts, Failure: existing.Failure}, nil
	}
	retryAt := existing.AttemptedAt.Add(retryAfter)
	if now.Before(retryAt) {
		return Decision{Disposition: DispositionPending, Attempts: existing.Attempts, RetryAt: retryAt}, nil
	}
	if existing.Attempts >= maxAttempts {
		existing.Status = IntentFailed
		if existing.Failure == "" {
			existing.Failure = fmt.Sprintf("rename submission failed after %d attempts", existing.Attempts)
		}
		if err := l.putLocked(key, existing); err != nil {
			return Decision{}, err
		}
		return Decision{
			Disposition: DispositionFailed,
			Attempts:    existing.Attempts,
			Failure:     existing.Failure,
		}, nil
	}

	existing.AttemptedAt = now
	existing.Attempts++
	existing.Status = IntentRetrying
	existing.Failure = ""
	if err := l.putLocked(key, existing); err != nil {
		return Decision{}, err
	}
	return Decision{
		Disposition: DispositionSubmit,
		Attempts:    existing.Attempts,
		RetryAt:     now.Add(retryAfter),
	}, nil
}

// MarkSubmitted records that the provider accepted the rename request.
func (l *Ledger) MarkSubmitted(key Key, serialNumber, desiredName string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	intent, found := l.intents[key]
	if !found || intent.SerialNumber != serialNumber || intent.DesiredName != desiredName {
		return nil
	}
	intent.Status = IntentSubmitted
	intent.Failure = ""
	return l.putLocked(key, intent)
}

// MarkRetrying records a transient provider failure for a later submission attempt.
func (l *Ledger) MarkRetrying(key Key, serialNumber, desiredName, reason string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	intent, found := l.intents[key]
	if !found || intent.SerialNumber != serialNumber || intent.DesiredName != desiredName {
		return nil
	}
	intent.Status = IntentRetrying
	intent.Failure = reason
	return l.putLocked(key, intent)
}

// MarkFailed records a non-retryable provider rejection if the completed request is still current.
func (l *Ledger) MarkFailed(key Key, serialNumber, desiredName, reason string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	intent, found := l.intents[key]
	if !found || intent.SerialNumber != serialNumber || intent.DesiredName != desiredName {
		return nil
	}
	intent.Status = IntentFailed
	intent.Failure = reason
	return l.putLocked(key, intent)
}

// Observe clears an intent once its desired name is visible, or when a provider ID points to a new serial.
func (l *Ledger) Observe(key Key, serialNumber, currentName string) (bool, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	intent, found := l.intents[key]
	if !found {
		return false, nil
	}
	if intent.SerialNumber != serialNumber || intent.DesiredName == currentName {
		if err := l.deleteLocked(key); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// Cancel removes an obsolete intent when policy no longer wants to rename a device.
func (l *Ledger) Cancel(key Key) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if _, found := l.intents[key]; !found {
		return nil
	}
	return l.deleteLocked(key)
}

// Snapshot returns intentions sorted by provider, resource namespace, and device ID.
func (l *Ledger) Snapshot() []Intent {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return snapshot(l.intents)
}

func (l *Ledger) putLocked(key Key, intent Intent) error {
	previous, found := l.intents[key]
	l.intents[key] = intent
	if err := l.store.Save(snapshot(l.intents)); err != nil {
		if found {
			l.intents[key] = previous
		} else {
			delete(l.intents, key)
		}
		return fmt.Errorf("save rename intent: %w", err)
	}
	return nil
}

func (l *Ledger) deleteLocked(key Key) error {
	previous := l.intents[key]
	delete(l.intents, key)
	if err := l.store.Save(snapshot(l.intents)); err != nil {
		l.intents[key] = previous
		return fmt.Errorf("save rename intent: %w", err)
	}
	return nil
}

func snapshot(intents map[Key]Intent) []Intent {
	result := make([]Intent, 0, len(intents))
	for _, intent := range intents {
		result = append(result, intent)
	}
	slices.SortFunc(result, func(left, right Intent) int {
		if order := cmp.Compare(left.Source, right.Source); order != 0 {
			return order
		}
		if order := cmp.Compare(left.Namespace, right.Namespace); order != 0 {
			return order
		}
		return cmp.Compare(left.DeviceID, right.DeviceID)
	})
	return result
}

func validateIntent(intent Intent) error {
	if intent.Source == "" || intent.Namespace == "" || intent.DeviceID == "" {
		return fmt.Errorf("source, namespace, and device_id are required")
	}
	if intent.SerialNumber == "" || intent.DesiredName == "" {
		return fmt.Errorf("serial_number and desired_name are required")
	}
	if intent.AttemptedAt.IsZero() || intent.Attempts <= 0 {
		return fmt.Errorf("attempted_at and a positive attempts count are required")
	}
	switch intent.Status {
	case IntentFailed, IntentRetrying, IntentSubmitted:
		return nil
	default:
		return fmt.Errorf("status %q is not supported", intent.Status)
	}
}

func validateRequest(
	key Key,
	serialNumber string,
	desiredName string,
	retryAfter time.Duration,
	maxAttempts int,
) error {
	if key.Source == "" || key.Namespace == "" || key.DeviceID == "" {
		return fmt.Errorf("source, namespace, and device ID are required")
	}
	if serialNumber == "" || desiredName == "" {
		return fmt.Errorf("serial number and desired name are required")
	}
	if retryAfter <= 0 || maxAttempts <= 0 {
		return fmt.Errorf("retry interval and maximum attempts must be greater than zero")
	}
	return nil
}
