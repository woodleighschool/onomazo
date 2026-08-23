package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/woodleighschool/onomazo/internal/expression"
)

const supportedVersion = 1

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func (c *Config) applyDefaults() {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if !c.Reconcile.PollInterval.set {
		c.Reconcile.PollInterval.Duration = time.Minute
	}
	if !c.Reconcile.DeviceDetailsTTL.set {
		c.Reconcile.DeviceDetailsTTL.Duration = time.Hour
	}
	if !c.Reconcile.IdentityTTL.set {
		c.Reconcile.IdentityTTL.Duration = 15 * time.Minute
	}
	if !c.Reconcile.RenameRetryAfter.set {
		c.Reconcile.RenameRetryAfter.Duration = 30 * time.Minute
	}
	if c.Reconcile.RenameMaxAttempts == 0 {
		c.Reconcile.RenameMaxAttempts = 3
	}
	if c.Reconcile.Concurrency == 0 {
		c.Reconcile.Concurrency = 4
	}
	if c.State.Type == "" {
		c.State.Type = "memory"
	}
	if c.Naming.Collisions.Disambiguate.Type == "" {
		c.Naming.Collisions.Disambiguate.Type = "sequence"
	}
	if c.Naming.Collisions.Disambiguate.Separator == "" {
		c.Naming.Collisions.Disambiguate.Separator = "-"
	}
}

func (c *Config) normalize() {
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	c.State.Type = strings.ToLower(strings.TrimSpace(c.State.Type))
	c.State.Path = strings.TrimSpace(c.State.Path)
}

func (c *Config) validateAndCompile() error {
	if c.Version != supportedVersion {
		return fmt.Errorf("config version must be %d, found %d", supportedVersion, c.Version)
	}
	if err := c.validateLogLevel(); err != nil {
		return err
	}
	if err := c.validateConnections(); err != nil {
		return err
	}
	if err := c.validateDeviceSources(); err != nil {
		return err
	}
	if err := c.validateIdentity(); err != nil {
		return err
	}
	if err := c.validateReconcile(); err != nil {
		return err
	}
	if err := c.validateState(); err != nil {
		return err
	}
	return c.validateNaming()
}

func (c *Config) validateLogLevel() error {
	switch c.LogLevel {
	case "debug":
		c.ParsedLevel = slog.LevelDebug
	case "info":
		c.ParsedLevel = slog.LevelInfo
	case "warn":
		c.ParsedLevel = slog.LevelWarn
	case "error":
		c.ParsedLevel = slog.LevelError
	default:
		return fmt.Errorf("log_level must be debug, info, warn, or error")
	}
	return nil
}

func (c *Config) validateConnections() error {
	if len(c.Connections) == 0 {
		return fmt.Errorf("connections must define at least one connection")
	}
	for _, name := range sortedKeys(c.Connections) {
		connection := c.Connections[name]
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("connections.%s: name must match %s", name, identifierPattern)
		}
		if strings.TrimSpace(connection.ClientID) == "" {
			return fmt.Errorf("connections.%s.client_id is required", name)
		}
		if strings.TrimSpace(connection.ClientSecret) == "" {
			return fmt.Errorf("connections.%s.client_secret is required", name)
		}
		switch connection.Type {
		case "microsoft_graph":
			if strings.TrimSpace(connection.TenantID) == "" {
				return fmt.Errorf("connections.%s.tenant_id is required", name)
			}
			if connection.URL != "" {
				return fmt.Errorf("connections.%s.url is only valid for jamf connections", name)
			}
		case "jamf":
			if err := validateHTTPURL(connection.URL); err != nil {
				return fmt.Errorf("connections.%s.url: %w", name, err)
			}
			if connection.TenantID != "" || connection.BaseURL != "" {
				return fmt.Errorf("connections.%s: tenant_id and base_url are only valid for microsoft_graph", name)
			}
		default:
			return fmt.Errorf("connections.%s.type %q is not supported", name, connection.Type)
		}
	}
	return nil
}

func (c *Config) validateDeviceSources() error {
	if len(c.Devices) == 0 {
		return fmt.Errorf("devices must define at least one source")
	}
	seenNames := make(map[string]struct{}, len(c.Devices))
	seenTypes := make(map[string]struct{}, len(c.Devices))
	for index, source := range c.Devices {
		path := fmt.Sprintf("devices[%d]", index)
		if !identifierPattern.MatchString(source.Name) {
			return fmt.Errorf("%s.name must match %s", path, identifierPattern)
		}
		if _, exists := seenNames[source.Name]; exists {
			return fmt.Errorf("%s.name %q is duplicated", path, source.Name)
		}
		seenNames[source.Name] = struct{}{}
		if _, exists := seenTypes[source.Type]; exists {
			return fmt.Errorf("%s.type %q is configured more than once", path, source.Type)
		}
		seenTypes[source.Type] = struct{}{}

		connection, ok := c.Connections[source.Connection]
		if !ok {
			return fmt.Errorf("%s.connection %q does not exist", path, source.Connection)
		}
		var supportedPlatforms []string
		switch source.Type {
		case "intune":
			if connection.Type != "microsoft_graph" {
				return fmt.Errorf("%s requires a microsoft_graph connection", path)
			}
			supportedPlatforms = []string{"ios", "macos", "windows"}
		case "jamf":
			if connection.Type != "jamf" {
				return fmt.Errorf("%s requires a jamf connection", path)
			}
			supportedPlatforms = []string{"ios", "macos"}
		default:
			return fmt.Errorf("%s.type %q is not supported", path, source.Type)
		}
		if len(source.Platforms) == 0 {
			return fmt.Errorf("%s.platforms must not be empty", path)
		}
		seenPlatforms := make(map[string]struct{}, len(source.Platforms))
		for _, platform := range source.Platforms {
			if !slices.Contains(supportedPlatforms, platform) {
				return fmt.Errorf("%s.platforms contains unsupported %s platform %q", path, source.Type, platform)
			}
			if _, exists := seenPlatforms[platform]; exists {
				return fmt.Errorf("%s.platforms contains duplicate %q", path, platform)
			}
			seenPlatforms[platform] = struct{}{}
		}
	}
	return nil
}

func (c *Config) validateIdentity() error {
	if c.Identity == nil {
		return nil
	}
	if !identifierPattern.MatchString(c.Identity.Name) {
		return fmt.Errorf("identity.name must match %s", identifierPattern)
	}
	if c.Identity.Type != "entra" {
		return fmt.Errorf("identity.type %q is not supported", c.Identity.Type)
	}
	connection, ok := c.Connections[c.Identity.Connection]
	if !ok {
		return fmt.Errorf("identity.connection %q does not exist", c.Identity.Connection)
	}
	if connection.Type != "microsoft_graph" {
		return fmt.Errorf("identity requires a microsoft_graph connection")
	}
	for _, alias := range sortedKeys(c.Identity.Groups) {
		if !identifierPattern.MatchString(alias) {
			return fmt.Errorf("identity.groups.%s: alias must match %s", alias, identifierPattern)
		}
		groupIDs := c.Identity.Groups[alias]
		if len(groupIDs) == 0 {
			return fmt.Errorf("identity.groups.%s must not be empty", alias)
		}
		seen := make(map[string]struct{}, len(groupIDs))
		for _, groupID := range groupIDs {
			if strings.TrimSpace(groupID) == "" {
				return fmt.Errorf("identity.groups.%s contains an empty group ID", alias)
			}
			if _, exists := seen[groupID]; exists {
				return fmt.Errorf("identity.groups.%s contains duplicate group ID", alias)
			}
			seen[groupID] = struct{}{}
		}
	}
	return nil
}

func (c *Config) validateReconcile() error {
	durations := []struct {
		name  string
		value time.Duration
	}{
		{"poll_interval", c.Reconcile.PollInterval.Duration},
		{"device_details_ttl", c.Reconcile.DeviceDetailsTTL.Duration},
		{"identity_ttl", c.Reconcile.IdentityTTL.Duration},
		{"rename_retry_after", c.Reconcile.RenameRetryAfter.Duration},
	}
	for _, duration := range durations {
		if duration.value <= 0 {
			return fmt.Errorf("reconcile.%s must be greater than zero", duration.name)
		}
	}
	if c.Reconcile.RenameMaxAttempts <= 0 {
		return fmt.Errorf("reconcile.rename_max_attempts must be greater than zero")
	}
	if c.Reconcile.Concurrency <= 0 {
		return fmt.Errorf("reconcile.concurrency must be greater than zero")
	}
	return nil
}

func (c *Config) validateState() error {
	switch c.State.Type {
	case "memory":
		if c.State.Path != "" {
			return fmt.Errorf("state.path is only valid for file state")
		}
	case "file":
		if strings.TrimSpace(c.State.Path) == "" {
			return fmt.Errorf("state.path is required for file state")
		}
	default:
		return fmt.Errorf("state.type %q is not supported", c.State.Type)
	}
	return nil
}

func (c *Config) validateNaming() error {
	if c.Naming.Constraints.MaxLength <= 0 {
		return fmt.Errorf("naming.constraints.max_length must be greater than zero")
	}
	if strings.TrimSpace(c.Naming.Constraints.Pattern) == "" {
		return fmt.Errorf("naming.constraints.pattern is required")
	}
	if _, err := regexp.Compile(c.Naming.Constraints.Pattern); err != nil {
		return fmt.Errorf("naming.constraints.pattern: %w", err)
	}

	compiler, err := expression.NewCompiler()
	if err != nil {
		return err
	}
	programs := Programs{Variables: make(map[string][]VariablePrograms, len(c.Naming.Variables))}
	if err := c.compileOverrides(compiler, &programs); err != nil {
		return err
	}
	if err := c.compileVariables(compiler, &programs); err != nil {
		return err
	}
	if err := c.compileRules(compiler, &programs); err != nil {
		return err
	}
	if err := c.compileCollisions(compiler, &programs); err != nil {
		return err
	}
	c.Programs = programs
	return nil
}

func (c *Config) compileOverrides(compiler *expression.Compiler, programs *Programs) error {
	seen := make(map[string]struct{}, len(c.Naming.Overrides))
	for index, override := range c.Naming.Overrides {
		path := fmt.Sprintf("naming.overrides[%d]", index)
		if err := validateUniqueName(path, override.Name, seen); err != nil {
			return err
		}
		hasExclude := override.Exclude != nil
		hasName := override.DesiredName != ""
		if hasExclude == hasName {
			return fmt.Errorf("%s must define exactly one of exclude: true or desired_name", path)
		}
		if hasExclude && !*override.Exclude {
			return fmt.Errorf("%s.exclude must be true when specified", path)
		}
		program, err := compiler.CompileCondition(override.When)
		if err != nil {
			return fmt.Errorf("%s.when: %w", path, err)
		}
		programs.Overrides = append(programs.Overrides, program)
	}
	return nil
}

func (c *Config) compileVariables(compiler *expression.Compiler, programs *Programs) error {
	for _, name := range sortedKeys(c.Naming.Variables) {
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("naming.variables.%s: name must match %s", name, identifierPattern)
		}
		variable := c.Naming.Variables[name]
		if len(variable.Cases) == 0 {
			return fmt.Errorf("naming.variables.%s.cases must not be empty", name)
		}
		compiled := make([]VariablePrograms, 0, len(variable.Cases))
		for index, variableCase := range variable.Cases {
			path := fmt.Sprintf("naming.variables.%s.cases[%d]", name, index)
			when, err := compiler.CompileCondition(variableCase.When)
			if err != nil {
				return fmt.Errorf("%s.when: %w", path, err)
			}
			hasValue := variableCase.Value != nil
			hasExpression := strings.TrimSpace(variableCase.Expression) != ""
			if hasValue == hasExpression {
				return fmt.Errorf("%s must define exactly one of value or expression", path)
			}
			program := VariablePrograms{When: when}
			if hasExpression {
				expressionProgram, err := compiler.CompileVariable(variableCase.Expression)
				if err != nil {
					return fmt.Errorf("%s.expression: %w", path, err)
				}
				program.Expression = &expressionProgram
			}
			compiled = append(compiled, program)
		}
		programs.Variables[name] = compiled
	}
	return nil
}

func (c *Config) compileRules(compiler *expression.Compiler, programs *Programs) error {
	if len(c.Naming.Rules) == 0 {
		return fmt.Errorf("naming.rules must define at least one rule")
	}
	seen := make(map[string]struct{}, len(c.Naming.Rules))
	for index, rule := range c.Naming.Rules {
		path := fmt.Sprintf("naming.rules[%d]", index)
		if err := validateUniqueName(path, rule.Name, seen); err != nil {
			return err
		}
		when, err := compiler.CompileRuleCondition(rule.When)
		if err != nil {
			return fmt.Errorf("%s.when: %w", path, err)
		}
		desiredName, err := compiler.CompileDesiredName(rule.DesiredName)
		if err != nil {
			return fmt.Errorf("%s.desired_name: %w", path, err)
		}
		programs.Rules = append(programs.Rules, RulePrograms{When: when, DesiredName: desiredName})
	}
	return nil
}

func (c *Config) compileCollisions(compiler *expression.Compiler, programs *Programs) error {
	for index, rank := range c.Naming.Collisions.Rank {
		path := fmt.Sprintf("naming.collisions.rank[%d]", index)
		if rank.Order != "ascending" && rank.Order != "descending" {
			return fmt.Errorf("%s.order must be ascending or descending", path)
		}
		program, err := compiler.CompileRank(rank.Expression)
		if err != nil {
			return fmt.Errorf("%s.expression: %w", path, err)
		}
		programs.Ranks = append(programs.Ranks, program)
	}
	if c.Naming.Collisions.Disambiguate.Type != "sequence" {
		return fmt.Errorf("naming.collisions.disambiguate.type must be sequence")
	}
	if strings.TrimSpace(c.Naming.Collisions.Disambiguate.Separator) == "" {
		return fmt.Errorf("naming.collisions.disambiguate.separator must not be empty")
	}
	return nil
}

func validateUniqueName(path, name string, seen map[string]struct{}) error {
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("%s.name must match %s", path, identifierPattern)
	}
	if _, exists := seen[name]; exists {
		return fmt.Errorf("%s.name %q is duplicated", path, name)
	}
	seen[name] = struct{}{}
	return nil
}

func validateHTTPURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("must be an absolute HTTP URL")
	}
	return nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
