package planner

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/expression"
)

const maximumSequence = 1_000_000

// Planner evaluates naming policy and resolves collisions over a complete inventory snapshot.
type Planner struct {
	config  *config.Config
	pattern *regexp.Regexp
}

type workItem struct {
	record        Record
	plan          Item
	candidate     bool
	authoritative bool
	ranks         []rankValue
}

type rankValue struct {
	kind      expression.Kind
	integer   int64
	text      string
	timestamp time.Time
}

// New creates a planner from an already validated and compiled configuration.
func New(cfg *config.Config) (*Planner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	pattern, err := regexp.Compile(cfg.Naming.Constraints.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile naming constraint: %w", err)
	}
	return &Planner{config: cfg, pattern: pattern}, nil
}

// Plan returns one stable plan item for every input record without performing remote writes.
func (p *Planner) Plan(records []Record) ([]Item, error) {
	work := make([]workItem, len(records))
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		key := record.Device.Source + "\x00" + record.Device.ID
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("device %s/%s is duplicated", record.Device.Source, record.Device.ID)
		}
		seen[key] = struct{}{}
		work[index] = p.evaluate(record)
	}

	slices.SortFunc(work, func(left, right workItem) int {
		if order := cmp.Compare(left.record.Device.Source, right.record.Device.Source); order != 0 {
			return order
		}
		return cmp.Compare(left.record.Device.ID, right.record.Device.ID)
	})
	p.evaluateRanks(work)
	p.resolveCollisions(work)

	plan := make([]Item, len(work))
	for index := range work {
		plan[index] = work[index].plan
	}
	return plan, nil
}

func (p *Planner) evaluate(record Record) workItem {
	item := workItem{
		record: record,
		plan: Item{
			Source:       record.Device.Source,
			ID:           record.Device.ID,
			SerialNumber: record.Device.SerialNumber,
			Platform:     record.Device.Platform,
			CurrentName:  record.Device.CurrentName,
			User:         record.User.UserPrincipalName,
		},
	}
	input := expression.Input{Device: &item.record.Device, User: &item.record.User}

	for index, override := range p.config.Naming.Overrides {
		matched, err := evalBool(p.config.Programs.Overrides[index], input)
		if err != nil {
			item.invalidate("override %q failed: %v", override.Name, err)
			return item
		}
		if !matched {
			continue
		}
		item.plan.Rule = override.Name
		if override.Exclude != nil {
			item.plan.Status = StatusExcluded
			item.plan.Reason = "matched override"
			return item
		}
		item.plan.DesiredName = override.DesiredName
		item.candidate = true
		item.authoritative = true
		if reason := p.invalidName(item.plan.DesiredName, item.record.Device.Platform); reason != "" {
			item.invalidate("fixed name %s", reason)
		}
		return item
	}

	variables, unresolved, err := p.evaluateVariables(input)
	if err != nil {
		item.invalidate("%v", err)
		return item
	}
	if unresolved != "" {
		item.plan.Status = StatusUnresolved
		item.plan.Reason = fmt.Sprintf("variable %q resolved to conflicting values", unresolved)
		return item
	}
	input.Variables = variables

	for index, rule := range p.config.Naming.Rules {
		matched, err := evalBool(p.config.Programs.Rules[index].When, input)
		if err != nil {
			item.invalidate("rule %q condition failed: %v", rule.Name, err)
			return item
		}
		if !matched {
			continue
		}
		desiredName, err := evalString(p.config.Programs.Rules[index].DesiredName, input)
		if err != nil {
			item.invalidate("rule %q desired name failed: %v", rule.Name, err)
			return item
		}
		item.plan.Rule = rule.Name
		item.plan.DesiredName = desiredName
		item.candidate = true
		if reason := p.invalidName(desiredName, item.record.Device.Platform); reason != "" {
			item.invalidate("desired name %s", reason)
		}
		return item
	}

	item.plan.Status = StatusUnmanaged
	item.plan.Reason = "no naming rule matched"
	return item
}

func (p *Planner) evaluateVariables(input expression.Input) (map[string]string, string, error) {
	variables := make(map[string]string)
	for _, name := range sortedKeys(p.config.Naming.Variables) {
		variable := p.config.Naming.Variables[name]
		values := make(map[string]struct{})
		for index, variableCase := range variable.Cases {
			programs := p.config.Programs.Variables[name][index]
			matched, err := evalBool(programs.When, input)
			if err != nil {
				return nil, "", fmt.Errorf("variable %q condition failed: %w", name, err)
			}
			if !matched {
				continue
			}
			var value string
			if variableCase.Value != nil {
				value = *variableCase.Value
			} else {
				value, err = evalString(*programs.Expression, input)
				if err != nil {
					return nil, "", fmt.Errorf("variable %q expression failed: %w", name, err)
				}
			}
			values[value] = struct{}{}
		}
		if len(values) > 1 {
			return variables, name, nil
		}
		for value := range values {
			variables[name] = value
		}
	}
	return variables, "", nil
}

func (p *Planner) evaluateRanks(work []workItem) {
	for index := range work {
		item := &work[index]
		if !item.candidate {
			continue
		}
		input := expression.Input{Device: &item.record.Device, User: &item.record.User}
		for rankIndex, program := range p.config.Programs.Ranks {
			value, err := evalRank(program, input)
			if err != nil {
				item.invalidate("collision rank %d failed: %v", rankIndex, err)
				break
			}
			item.ranks = append(item.ranks, value)
		}
	}
}

func (p *Planner) resolveCollisions(work []workItem) {
	reserved := make(map[string]struct{})
	groups := make(map[string][]*workItem)
	for index := range work {
		item := &work[index]
		if !item.candidate {
			reserve(reserved, item.plan.CurrentName)
			continue
		}
		key := foldName(item.plan.DesiredName)
		groups[key] = append(groups[key], item)
	}

	groupKeys := sortedKeys(groups)
	winners := make(map[string]*workItem, len(groups))
	for _, key := range groupKeys {
		group := groups[key]
		authoritative := authoritativeItems(group)
		_, isReserved := reserved[key]
		switch {
		case len(authoritative) > 1:
			for _, item := range group {
				item.invalidate("authoritative name conflicts with another fixed name")
				reserve(reserved, item.plan.CurrentName)
			}
		case len(authoritative) == 1 && isReserved:
			authoritative[0].invalidate("authoritative name conflicts with an existing device")
			reserve(reserved, authoritative[0].plan.CurrentName)
		case len(authoritative) == 1:
			winners[key] = authoritative[0]
		case !isReserved:
			sort.SliceStable(group, func(left, right int) bool {
				return p.compareCandidates(group[left], group[right]) < 0
			})
			winners[key] = group[0]
		}
	}

	occupied := make(map[string]struct{}, len(reserved)+len(winners))
	for key := range reserved {
		occupied[key] = struct{}{}
	}
	for key, winner := range winners {
		if winner.candidate {
			occupied[key] = struct{}{}
			winner.finish()
		}
	}

	for _, key := range groupKeys {
		group := groups[key]
		sort.SliceStable(group, func(left, right int) bool {
			return p.compareCandidates(group[left], group[right]) < 0
		})
		for _, item := range group {
			if !item.candidate || winners[key] == item {
				continue
			}
			if item.authoritative {
				item.invalidate("authoritative name cannot be suffixed")
				reserve(occupied, item.plan.CurrentName)
				continue
			}
			name, reason := p.nextSequenceName(item.plan.DesiredName, item.record.Device.Platform, occupied)
			if reason != "" {
				item.invalidate("collision %s", reason)
				reserve(occupied, item.plan.CurrentName)
				continue
			}
			item.plan.DesiredName = name
			occupied[foldName(name)] = struct{}{}
			item.finish()
		}
	}
}

func (p *Planner) compareCandidates(left, right *workItem) int {
	if p.config.Naming.Collisions.Disambiguate.PreserveExisting {
		leftExisting := foldName(left.plan.CurrentName) == foldName(left.plan.DesiredName)
		rightExisting := foldName(right.plan.CurrentName) == foldName(right.plan.DesiredName)
		if leftExisting != rightExisting {
			if leftExisting {
				return -1
			}
			return 1
		}
	}
	for index, leftRank := range left.ranks {
		order := compareRank(leftRank, right.ranks[index])
		if p.config.Naming.Collisions.Rank[index].Order == "descending" {
			order = -order
		}
		if order != 0 {
			return order
		}
	}
	if order := cmp.Compare(left.record.Device.Source, right.record.Device.Source); order != 0 {
		return order
	}
	return cmp.Compare(left.record.Device.ID, right.record.Device.ID)
}

func (p *Planner) nextSequenceName(base, platform string, occupied map[string]struct{}) (string, string) {
	separator := p.config.Naming.Collisions.Disambiguate.Separator
	for sequence := 1; sequence <= maximumSequence; sequence++ {
		candidate := base + separator + strconv.Itoa(sequence)
		if reason := p.invalidName(candidate, platform); reason != "" {
			if len(candidate) > p.maximumLength(platform) {
				return "", "suffix exceeds the maximum length"
			}
			continue
		}
		if _, exists := occupied[foldName(candidate)]; !exists {
			return candidate, ""
		}
	}
	return "", "sequence space is exhausted"
}

func (p *Planner) invalidName(name, platform string) string {
	if name == "" {
		return "is empty"
	}
	if len(name) > p.maximumLength(platform) {
		return fmt.Sprintf("exceeds maximum length %d", p.maximumLength(platform))
	}
	if !p.pattern.MatchString(name) {
		return "does not match the configured pattern"
	}
	return ""
}

func (p *Planner) maximumLength(platform string) int {
	maximum := p.config.Naming.Constraints.MaxLength
	switch platform {
	case "windows":
		return min(maximum, 15)
	case "ios", "macos":
		return min(maximum, 63)
	default:
		return maximum
	}
}

func (item *workItem) invalidate(format string, values ...any) {
	item.candidate = false
	item.plan.Status = StatusInvalid
	item.plan.Reason = fmt.Sprintf(format, values...)
}

func (item *workItem) finish() {
	if item.plan.CurrentName == item.plan.DesiredName {
		item.plan.Status = StatusUnchanged
		item.plan.Reason = "name already matches"
		return
	}
	item.plan.Status = StatusRename
	item.plan.Reason = "name differs"
}

func evalBool(program expression.Program, input expression.Input) (bool, error) {
	value, err := program.Eval(input)
	if err != nil {
		return false, err
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("result is %T, want bool", value)
	}
	return result, nil
}

func evalString(program expression.Program, input expression.Input) (string, error) {
	value, err := program.Eval(input)
	if err != nil {
		return "", err
	}
	result, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("result is %T, want string", value)
	}
	return result, nil
}

func evalRank(program expression.Program, input expression.Input) (rankValue, error) {
	value, err := program.Eval(input)
	if err != nil {
		return rankValue{}, err
	}
	result := rankValue{kind: program.Kind()}
	switch program.Kind() {
	case expression.KindInt:
		integer, ok := value.(int64)
		if !ok {
			return rankValue{}, fmt.Errorf("rank result is %T, want int64", value)
		}
		result.integer = integer
	case expression.KindString:
		text, ok := value.(string)
		if !ok {
			return rankValue{}, fmt.Errorf("rank result is %T, want string", value)
		}
		result.text = text
	case expression.KindTimestamp:
		timestamp, ok := value.(time.Time)
		if !ok {
			return rankValue{}, fmt.Errorf("rank result is %T, want time.Time", value)
		}
		result.timestamp = timestamp
	default:
		return rankValue{}, fmt.Errorf("unsupported rank type %s", program.Kind())
	}
	return result, nil
}

func compareRank(left, right rankValue) int {
	switch left.kind {
	case expression.KindInt:
		return cmp.Compare(left.integer, right.integer)
	case expression.KindString:
		return cmp.Compare(left.text, right.text)
	case expression.KindTimestamp:
		return left.timestamp.Compare(right.timestamp)
	default:
		return 0
	}
}

func authoritativeItems(items []*workItem) []*workItem {
	result := make([]*workItem, 0, len(items))
	for _, item := range items {
		if item.authoritative {
			result = append(result, item)
		}
	}
	return result
}

func reserve(names map[string]struct{}, name string) {
	if name != "" {
		names[foldName(name)] = struct{}{}
	}
}

func foldName(name string) string {
	return strings.ToUpper(name)
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
