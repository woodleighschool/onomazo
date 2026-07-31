package state

import "time"

// Key identifies one device within a provider.
type Key struct {
	Source   string
	DeviceID string
}

// IntentStatus describes an outstanding rename intention.
type IntentStatus string

const (
	IntentPending IntentStatus = "pending"
	IntentFailed  IntentStatus = "failed"
	IntentStalled IntentStatus = "stalled"
)

// Intent records a rename before the corresponding provider request is submitted.
type Intent struct {
	Source       string       `json:"source"`
	DeviceID     string       `json:"device_id"`
	SerialNumber string       `json:"serial_number"`
	DesiredName  string       `json:"desired_name"`
	AttemptedAt  time.Time    `json:"attempted_at"`
	Attempts     int          `json:"attempts"`
	Status       IntentStatus `json:"status"`
	Failure      string       `json:"failure,omitempty"`
}

// Disposition tells the reconciler whether a rename should be submitted now.
type Disposition string

const (
	DispositionFailed   Disposition = "failed"
	DispositionObserved Disposition = "observed"
	DispositionPending  Disposition = "pending"
	DispositionStalled  Disposition = "stalled"
	DispositionSubmit   Disposition = "submit"
)

// Decision is the ledger result for one desired rename.
type Decision struct {
	Disposition Disposition
	Attempts    int
	RetryAt     time.Time
	Failure     string
}
