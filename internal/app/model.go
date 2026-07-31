package app

import (
	"context"
	"time"

	"github.com/woodleighschool/onomazo/internal/domain"
	"github.com/woodleighschool/onomazo/internal/planner"
)

// DeviceSource provides one named inventory and its rename operation.
type DeviceSource interface {
	Name() string
	ListDevices(context.Context) ([]domain.Device, error)
	Rename(context.Context, domain.Device, string) error
	StatusCode(error) (int, bool)
}

// IdentityResolver enriches only the user identifiers present in the current inventory.
type IdentityResolver interface {
	ResolveUsers(context.Context, []string, map[string][]string, int) (map[string]domain.User, error)
}

type namePlanner interface {
	Plan([]planner.Record) ([]planner.Item, error)
}

// BuildMode selects whether file state needs an exclusive writer lock.
type BuildMode string

const (
	BuildReadOnly BuildMode = "read_only"
	BuildApply    BuildMode = "apply"
)

// Action describes what the reconciliation layer did with a plan entry.
type Action string

const (
	ActionFailed    Action = "failed"
	ActionPending   Action = "pending"
	ActionPlanned   Action = "planned"
	ActionStalled   Action = "stalled"
	ActionSubmitted Action = "submitted"
)

// Result combines a deterministic naming decision with its remote-action state.
type Result struct {
	planner.Item
	Action   Action    `json:"action,omitempty"`
	Attempts int       `json:"attempts,omitempty"`
	RetryAt  time.Time `json:"retry_at,omitzero"`
	Error    string    `json:"error,omitempty"`
}

// Options contains the dependencies and timing policy for one service.
type Options struct {
	Sources           []DeviceSource
	Identity          IdentityResolver
	GroupAliases      map[string][]string
	Planner           namePlanner
	Ledger            ledger
	DeviceDetailsTTL  time.Duration
	IdentityTTL       time.Duration
	RenameRetryAfter  time.Duration
	RenameMaxAttempts int
	Concurrency       int
	Now               func() time.Time
}
