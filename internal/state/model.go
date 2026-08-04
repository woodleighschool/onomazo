package state

import "time"

// Key identifies one device within a provider resource namespace.
type Key struct {
	Source    string
	Namespace string
	DeviceID  string
}

// IntentStatus describes an outstanding rename intention.
type IntentStatus string

const (
	IntentFailed    IntentStatus = "failed"
	IntentRetrying  IntentStatus = "retrying"
	IntentSubmitted IntentStatus = "submitted"
)

// Intent records an outstanding rename request and its submission state.
type Intent struct {
	Source       string       `json:"source"`
	Namespace    string       `json:"namespace"`
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
	DispositionSubmit   Disposition = "submit"
)

// Decision is the ledger result for one desired rename.
type Decision struct {
	Disposition Disposition
	Attempts    int
	RetryAt     time.Time
	Failure     string
}
