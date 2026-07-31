package config

import (
	"time"

	"github.com/woodleighschool/onomazo/internal/expression"
)

// Config is Onomazo's complete versioned configuration.
type Config struct {
	Version     int                   `yaml:"version"`
	Connections map[string]Connection `yaml:"connections"`
	Devices     []DeviceSource        `yaml:"devices"`
	Identity    *Identity             `yaml:"identity,omitempty"`
	Reconcile   Reconcile             `yaml:"reconcile"`
	State       State                 `yaml:"state,omitempty"`
	Naming      Naming                `yaml:"naming"`
	Programs    Programs              `yaml:"-"`
}

// Connection contains credentials for a remote API.
type Connection struct {
	Type         string `yaml:"type"`
	TenantID     string `yaml:"tenant_id,omitempty"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	URL          string `yaml:"url,omitempty"`
	BaseURL      string `yaml:"base_url,omitempty"`
}

// DeviceSource selects a managed-device provider and its supported platforms.
type DeviceSource struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	Connection string   `yaml:"connection"`
	Platforms  []string `yaml:"platforms"`
}

// Identity selects the user directory used to enrich associated users.
type Identity struct {
	Name       string              `yaml:"name"`
	Type       string              `yaml:"type"`
	Connection string              `yaml:"connection"`
	Groups     map[string][]string `yaml:"groups,omitempty"`
}

// Reconcile controls polling, cache lifetimes, and rename retry behavior.
type Reconcile struct {
	PollInterval      Duration `yaml:"poll_interval"`
	DeviceDetailsTTL  Duration `yaml:"device_details_ttl"`
	IdentityTTL       Duration `yaml:"identity_ttl"`
	RenameRetryAfter  Duration `yaml:"rename_retry_after"`
	RenameMaxAttempts int      `yaml:"rename_max_attempts"`
	Concurrency       int      `yaml:"concurrency"`
}

// State selects the rename-intent store. Memory state remains fully functional but does not survive restarts.
type State struct {
	Type string `yaml:"type"`
	Path string `yaml:"path,omitempty"`
}

// Naming contains the ordered policy evaluated for every discovered device.
type Naming struct {
	Constraints Constraints         `yaml:"constraints"`
	Overrides   []Override          `yaml:"overrides,omitempty"`
	Variables   map[string]Variable `yaml:"variables,omitempty"`
	Rules       []Rule              `yaml:"rules"`
	Collisions  Collisions          `yaml:"collisions"`
}

// Constraints are applied to every final name before any rename is submitted.
type Constraints struct {
	MaxLength int    `yaml:"max_length"`
	Pattern   string `yaml:"pattern"`
}

// Override excludes a device or gives it an authoritative literal name.
type Override struct {
	Name        string `yaml:"name"`
	When        string `yaml:"when"`
	Exclude     *bool  `yaml:"exclude,omitempty"`
	DesiredName string `yaml:"desired_name,omitempty"`
}

// Variable derives a reusable string from all matching cases.
type Variable struct {
	Cases []VariableCase `yaml:"cases"`
}

// VariableCase yields either a literal value or a CEL string expression.
type VariableCase struct {
	When       string  `yaml:"when"`
	Value      *string `yaml:"value,omitempty"`
	Expression string  `yaml:"expression,omitempty"`
}

// Rule is an ordered CEL condition and desired-name expression.
type Rule struct {
	Name        string `yaml:"name"`
	When        string `yaml:"when"`
	DesiredName string `yaml:"desired_name"`
}

// Collisions defines deterministic rank and disambiguation behavior.
type Collisions struct {
	Rank         []Rank         `yaml:"rank,omitempty"`
	Disambiguate Disambiguation `yaml:"disambiguate"`
}

// Rank is one comparable CEL expression and its sort direction.
type Rank struct {
	Expression string `yaml:"expression"`
	Order      string `yaml:"order"`
}

// Disambiguation selects how non-authoritative duplicate names receive sequence suffixes.
type Disambiguation struct {
	Type             string `yaml:"type"`
	Separator        string `yaml:"separator"`
	PreserveExisting bool   `yaml:"preserve_existing"`
}

// Programs holds CEL programs compiled while loading the configuration.
type Programs struct {
	Overrides []expression.Program
	Variables map[string][]VariablePrograms
	Rules     []RulePrograms
	Ranks     []expression.Program
}

// VariablePrograms contains the compiled expressions for one variable case.
type VariablePrograms struct {
	When       expression.Program
	Expression *expression.Program
}

// RulePrograms contains the compiled expressions for one rule.
type RulePrograms struct {
	When        expression.Program
	DesiredName expression.Program
}

// Duration is a YAML duration parsed with time.ParseDuration.
type Duration struct {
	time.Duration
	set bool
}
