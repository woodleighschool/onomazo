package naming

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"text/template"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/intune"
)

// Engine generates device names from configuration rules.
type Engine struct {
	rules         []*Rule
	overlays      []*metadataOverlay
	overrides     map[string]staticOverride
	exclusions    map[string]struct{}
	defaultPolicy duplicatePolicy
	globalMax     int
	funcs         template.FuncMap
	logger        *slog.Logger
}

type staticOverride struct {
	name    string
	enforce bool
}

type metadataOverlay struct {
	name     string
	priority int
	order    int
	matcher  *Matcher
	anyGroup []string
	allGroup []string
	values   map[string]string
}

// Decision represents a device naming decision.
type Decision struct {
	DeviceID     string
	SerialNumber string
	CurrentName  string
	DesiredName  string
	RuleName     string
	Reason       string
	ShouldUpdate bool
	Static       bool
	MetadataTags []string
}

func NewEngine(cfg *config.Config, maxDeviceNameLen int, logger *slog.Logger) (*Engine, error) {
	if maxDeviceNameLen <= 0 {
		maxDeviceNameLen = 63
	}
	funcs := templateFuncMap()
	defaultPolicy := policyFromConfig(cfg.Settings.DuplicatePolicy)
	if logger == nil {
		logger = slog.Default()
	}

	rules := make([]*Rule, 0, len(cfg.RuleDefinitions))
	for idx, rc := range cfg.RuleDefinitions {
		matcher, err := newMatcher(rc.Match)
		if err != nil {
			return nil, fmt.Errorf("rule[%d] matcher: %w", idx, err)
		}
		tmpl, err := template.New(rc.Name).Funcs(funcs).Parse(rc.Template)
		if err != nil {
			return nil, fmt.Errorf("rule[%d] template: %w", idx, err)
		}
		policy := defaultPolicy
		if rc.DuplicatePolicy != nil {
			policy = policyFromConfig(*rc.DuplicatePolicy)
		}
		rules = append(rules, &Rule{
			name:           rc.Name,
			priority:       rc.Priority,
			order:          idx,
			matcher:        matcher,
			template:       tmpl,
			maxLen:         chooseLength(rc.MaxLength, maxDeviceNameLen),
			stopProcessing: rc.StopProcessing,
			policy:         policy,
		})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].priority == rules[j].priority {
			return rules[i].order < rules[j].order
		}
		return rules[i].priority > rules[j].priority
	})

	metadata := make([]*metadataOverlay, 0, len(cfg.MetadataOverlays))
	for idx, mc := range cfg.MetadataOverlays {
		matcher, err := newMatcher(mc.Match.Attributes)
		if err != nil {
			return nil, fmt.Errorf("metadata[%d]: %w", idx, err)
		}
		values := make(map[string]string, len(mc.Values))
		for k, v := range mc.Values {
			values[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
		metadata = append(metadata, &metadataOverlay{
			name:     mc.Name,
			priority: mc.Priority,
			order:    idx,
			matcher:  matcher,
			anyGroup: normaliseIDs(mc.Match.AnyGroup),
			allGroup: normaliseIDs(mc.Match.AllGroup),
			values:   values,
		})
	}
	sort.SliceStable(metadata, func(i, j int) bool {
		if metadata[i].priority == metadata[j].priority {
			return metadata[i].order < metadata[j].order
		}
		return metadata[i].priority > metadata[j].priority
	})

	overrideMap := make(map[string]staticOverride, len(cfg.StaticNames))
	for _, entry := range cfg.StaticNames {
		serial := strings.ToLower(strings.TrimSpace(entry.Serial))
		name := strings.TrimSpace(entry.Name)
		if serial == "" || name == "" {
			continue
		}
		overrideMap[serial] = staticOverride{name: name, enforce: entry.Enforce}
	}

	excludes := make(map[string]struct{}, len(cfg.Exclude.Serials))
	for _, serial := range cfg.Exclude.Serials {
		serial = strings.ToLower(strings.TrimSpace(serial))
		if serial == "" {
			continue
		}
		excludes[serial] = struct{}{}
	}

	return &Engine{
		rules:         rules,
		overlays:      metadata,
		overrides:     overrideMap,
		exclusions:    excludes,
		defaultPolicy: defaultPolicy,
		globalMax:     maxDeviceNameLen,
		funcs:         funcs,
		logger:        logger,
	}, nil
}

// Decide determines the appropriate name for a device.
func (e *Engine) Decide(device *intune.ManagedDevice, user *intune.UserProfile, registry *NameRegistry) (Decision, error) {
	ctx := newDeviceContext(device, user)
	e.applyMetadata(ctx)
	deviceLogger := e.loggerForDevice(device)

	decision := Decision{
		DeviceID:     device.ID,
		SerialNumber: strings.ToUpper(device.SerialNumber),
		CurrentName:  device.DeviceName,
		MetadataTags: ctx.AppliedOverlays(),
	}

	serial := strings.ToLower(strings.TrimSpace(device.SerialNumber))
	if serial != "" {
		if _, found := e.exclusions[serial]; found {
			decision.Reason = "excluded serial"
			return decision, nil
		}
		if override, found := e.overrides[serial]; found {
			desired := e.enforceLength(override.name, e.globalMax)
			decision.DesiredName = desired
			decision.RuleName = "static:" + decision.SerialNumber
			decision.Static = true

			shouldRename := override.enforce || strings.TrimSpace(decision.CurrentName) == ""
			if shouldRename {
				decision.ShouldUpdate = !strings.EqualFold(decision.CurrentName, desired)
			}
			if decision.ShouldUpdate {
				decision.Reason = "static override"
				if registry != nil {
					registry.Update(ctx, desired)
				}
			} else if strings.EqualFold(decision.CurrentName, desired) {
				decision.Reason = "already static"
				if registry != nil {
					registry.Update(ctx, desired)
				}
			} else {
				decision.Reason = "static not enforced"
			}
			return decision, nil
		}
	}

	name, matchedRule, err := e.evaluateRules(ctx, deviceLogger)
	if err != nil {
		return decision, err
	}
	if matchedRule == nil || name == "" {
		decision.Reason = "no matching rule"
		return decision, nil
	}

	finalName := name
	if registry != nil {
		resolved, skipped, err := matchedRule.policy.resolve(ctx, name, matchedRule.maxLen, registry)
		if err != nil {
			return decision, err
		}
		if skipped {
			decision.Reason = "duplicate conflict"
			return decision, nil
		}
		finalName = resolved
	}
	finalName = e.enforceLength(finalName, e.globalMax)

	decision.DesiredName = finalName
	decision.RuleName = matchedRule.name
	decision.ShouldUpdate = !strings.EqualFold(decision.CurrentName, decision.DesiredName)
	if decision.ShouldUpdate {
		decision.Reason = "rule matched"
		if registry != nil {
			registry.Update(ctx, decision.DesiredName)
		}
	} else {
		decision.Reason = "already compliant"
		if registry != nil && decision.DesiredName != "" && strings.EqualFold(decision.CurrentName, decision.DesiredName) {
			registry.Update(ctx, decision.DesiredName)
		}
	}

	return decision, nil
}

func (e *Engine) applyMetadata(ctx *DeviceContext) {
	if len(e.overlays) == 0 {
		return
	}
	claimed := make(map[string]struct{})
	attrs := ctx.Attributes()
	for _, overlay := range e.overlays {
		if !overlay.matches(ctx, attrs) {
			continue
		}
		for key, value := range overlay.values {
			if value == "" {
				continue
			}
			if _, exists := claimed[key]; exists {
				continue
			}
			ctx.setAttr(key, value)
			claimed[key] = struct{}{}
		}
		ctx.markOverlay(overlay.name)
	}
}

func (m *metadataOverlay) matches(ctx *DeviceContext, attrs map[string]string) bool {
	if len(m.anyGroup) > 0 {
		matched := false
		for _, id := range m.anyGroup {
			if ctx.hasGroup(id) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(m.allGroup) > 0 {
		for _, id := range m.allGroup {
			if !ctx.hasGroup(id) {
				return false
			}
		}
	}
	if m.matcher != nil && !m.matcher.Matches(attrs) {
		return false
	}
	return true
}

func (e *Engine) evaluateRules(ctx *DeviceContext, log *slog.Logger) (string, *Rule, error) {
	var output string
	var matched *Rule
	logger := log
	if logger == nil {
		logger = e.logger
	}
	if logger == nil {
		logger = slog.Default()
	}
	for _, rule := range e.rules {
		if matched, reason := rule.matcher.MatchesWithReason(ctx.Attributes()); !matched {
			if reason != "" {
				logger.Debug("skip rule: matcher mismatch", "rule", rule.name, "reason", reason)
			} else {
				logger.Debug("skip rule: matcher mismatch", "rule", rule.name)
			}
			continue
		}
		rendered, err := rule.render(ctx)
		if err != nil {
			var miss missingAttrError
			if errors.As(err, &miss) {
				logger.Debug("skip rule: missing attribute", "rule", rule.name, "attribute", miss.Attribute())
				continue
			}
			logger.Warn("rule template error", "rule", rule.name, "error", err)
			continue
		}
		if rendered == "" {
			continue
		}
		output = rendered
		matched = rule
		if rule.stopProcessing {
			break
		}
	}
	if matched == nil {
		return "", nil, nil
	}
	return output, matched, nil
}

func (e *Engine) enforceLength(name string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = e.globalMax
	}
	name = strings.TrimSpace(name)
	if maxLen == 0 || len(name) <= maxLen {
		return name
	}
	return name[:maxLen]
}

type Rule struct {
	name           string
	priority       int
	order          int
	matcher        *Matcher
	template       *template.Template
	maxLen         int
	stopProcessing bool
	policy         duplicatePolicy
}

func (r *Rule) render(ctx *DeviceContext) (string, error) {
	var buf bytes.Buffer
	if err := r.template.Execute(&buf, ctx); err != nil {
		return "", err
	}
	output := strings.TrimSpace(buf.String())
	if r.maxLen > 0 && len(output) > r.maxLen {
		output = output[:r.maxLen]
	}
	return output, nil
}

func chooseLength(ruleLen, global int) int {
	if ruleLen > 0 {
		return ruleLen
	}
	return global
}

func normaliseIDs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		id := strings.ToLower(strings.TrimSpace(v))
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (e *Engine) loggerForDevice(device *intune.ManagedDevice) *slog.Logger {
	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	if device == nil {
		return logger
	}
	serial := strings.ToUpper(strings.TrimSpace(device.SerialNumber))
	return logger.With(
		"deviceId", device.ID,
		"serial", serial,
		"current", strings.TrimSpace(device.DeviceName),
	)
}
