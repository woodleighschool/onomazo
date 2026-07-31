package config

import (
	"time"

	"github.com/woodleighschool/onomazo/internal/expression"
)

// Config is Onomazo's complete versioned configuration.
type Config struct {
	Version     int                   `yaml:"version"     jsonschema:"enum=1"`
	Connections map[string]Connection `yaml:"connections" jsonschema:"minProperties=1"`
	Devices     []DeviceSource        `yaml:"devices"     jsonschema:"minItems=1"`
	Identity    *Identity             `yaml:"identity,omitempty"`
	Reconcile   Reconcile             `yaml:"reconcile,omitempty"`
	State       State                 `yaml:"state,omitempty"`
	Naming      Naming                `yaml:"naming"`
	Programs    Programs              `yaml:"-"`
}

// Connection contains credentials for a remote API.
type Connection struct {
	Type         string `yaml:"type"                    jsonschema:"enum=microsoft_graph,enum=jamf"`
	TenantID     string `yaml:"tenant_id,omitempty"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	URL          string `yaml:"url,omitempty"`
	BaseURL      string `yaml:"base_url,omitempty"`
}

// DeviceSource selects a managed-device provider and its supported platforms.
type DeviceSource struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"       jsonschema:"enum=intune,enum=jamf"`
	Connection string   `yaml:"connection"`
	Platforms  []string `yaml:"platforms"  jsonschema:"minItems=1,uniqueItems=true"`
}

// Identity selects the user directory used to enrich associated users.
type Identity struct {
	Name       string              `yaml:"name"`
	Type       string              `yaml:"type"             jsonschema:"enum=entra"`
	Connection string              `yaml:"connection"`
	Groups     map[string][]string `yaml:"groups,omitempty"`
}

// Reconcile controls polling, cache lifetimes, and rename retry behavior.
type Reconcile struct {
	PollInterval      Duration `yaml:"poll_interval,omitempty"`
	DeviceDetailsTTL  Duration `yaml:"device_details_ttl,omitempty"`
	IdentityTTL       Duration `yaml:"identity_ttl,omitempty"`
	RenameRetryAfter  Duration `yaml:"rename_retry_after,omitempty"`
	RenameMaxAttempts int      `yaml:"rename_max_attempts,omitempty" jsonschema:"minimum=1"`
	Concurrency       int      `yaml:"concurrency,omitempty"         jsonschema:"minimum=1"`
}

// State selects the rename-intent store. Memory state remains fully functional but does not survive restarts.
type State struct {
	Type string `yaml:"type,omitempty" jsonschema:"enum=memory,enum=file"`
	Path string `yaml:"path,omitempty"`
}

// Naming contains the ordered policy evaluated for every discovered device.
type Naming struct {
	Constraints Constraints         `yaml:"constraints"`
	Overrides   []Override          `yaml:"overrides,omitempty"`
	Variables   map[string]Variable `yaml:"variables,omitempty"`
	Rules       []Rule              `yaml:"rules"               jsonschema:"minItems=1"`
	Collisions  Collisions          `yaml:"collisions,omitempty"`
}

// Constraints are applied to every final name before any rename is submitted.
type Constraints struct {
	MaxLength int    `yaml:"max_length" jsonschema:"minimum=1"`
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
	Disambiguate Disambiguation `yaml:"disambiguate,omitempty"`
}

// Rank is one comparable CEL expression and its sort direction.
type Rank struct {
	Expression string `yaml:"expression"`
	Order      string `yaml:"order"      jsonschema:"enum=ascending,enum=descending"`
}

// Disambiguation selects how non-authoritative duplicate names receive sequence suffixes.
type Disambiguation struct {
	Type             string `yaml:"type,omitempty"              jsonschema:"enum=sequence"`
	Separator        string `yaml:"separator,omitempty"`
	PreserveExisting bool   `yaml:"preserve_existing,omitempty"`
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
