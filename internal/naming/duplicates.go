package naming

import (
	"fmt"
	"strings"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/intune"
)

type duplicateScope int

type duplicateStrategy int

const (
	scopeGlobal duplicateScope = iota
	scopePerUser
	scopePerPlatform
)

const (
	strategyAppendSuffix duplicateStrategy = iota
	strategySkip
	strategyError
	strategyOverwrite
)

type duplicatePolicy struct {
	scope        duplicateScope
	strategy     duplicateStrategy
	suffixFormat string
	suffixMin    int
	suffixMax    int
}

func policyFromConfig(cfg config.DuplicatePolicy) duplicatePolicy {
	return duplicatePolicy{
		scope:        parseScope(cfg.Scope),
		strategy:     parseStrategy(cfg.OnConflict),
		suffixFormat: cfg.Suffix.Format,
		suffixMin:    cfg.Suffix.Min,
		suffixMax:    cfg.Suffix.Max,
	}
}

func parseScope(scope string) duplicateScope {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "per-user":
		return scopePerUser
	case "per-platform":
		return scopePerPlatform
	default:
		return scopeGlobal
	}
}

func parseStrategy(strategy string) duplicateStrategy {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "skip":
		return strategySkip
	case "error":
		return strategyError
	case "overwrite":
		return strategyOverwrite
	default:
		return strategyAppendSuffix
	}
}

func (p duplicatePolicy) resolve(ctx *DeviceContext, base string, limit int, registry *NameRegistry) (string, bool, error) {
	if registry == nil || ctx == nil || ctx.Device == nil {
		return base, false, nil
	}
	if p.strategy == strategyOverwrite {
		return base, false, nil
	}
	scopeKey := scopeKeyFromContext(p.scope, ctx)
	if !registry.IsTaken(p.scope, scopeKey, base, ctx.Device.ID) {
		return base, false, nil
	}
	switch p.strategy {
	case strategySkip:
		return base, true, nil
	case strategyError:
		return "", false, fmt.Errorf("duplicate name %q already exists", base)
	case strategyAppendSuffix:
		for i := p.suffixMin; i <= p.suffixMax; i++ {
			suffix := fmt.Sprintf(p.suffixFormat, i)
			candidateBase := base
			if limit > 0 {
				candidateBase = truncateForSuffix(candidateBase, suffix, limit)
			}
			candidate := candidateBase + suffix
			if !registry.IsTaken(p.scope, scopeKey, candidate, ctx.Device.ID) {
				return candidate, false, nil
			}
		}
		return "", false, fmt.Errorf("exhausted suffixes for %q", base)
	default:
		return base, false, nil
	}
}

func truncateForSuffix(base, suffix string, limit int) string {
	if limit <= 0 {
		return base
	}
	available := limit - len(suffix)
	if available < 0 {
		available = 0
	}
	if len(base) <= available {
		return base
	}
	return base[:available]
}

// NameRegistry tracks device names to prevent duplicates.
type NameRegistry struct {
	global      map[string]string
	perPlatform map[string]map[string]string
	perUser     map[string]map[string]string
	claims      map[string][]claimRecord
}

type claimRecord struct {
	scope duplicateScope
	key   string
	name  string
}

func NewNameRegistry(devices []intune.ManagedDevice) *NameRegistry {
	registry := &NameRegistry{
		global:      make(map[string]string),
		perPlatform: make(map[string]map[string]string),
		perUser:     make(map[string]map[string]string),
		claims:      make(map[string][]claimRecord),
	}
	for i := range devices {
		registry.seed(&devices[i])
	}
	return registry
}

func (r *NameRegistry) seed(device *intune.ManagedDevice) {
	if r == nil || device == nil {
		return
	}
	name := normaliseName(device.DeviceName)
	if name == "" || device.ID == "" {
		return
	}
	r.claim(scopeGlobal, "", name, device.ID)
	if key := scopeKeyFromDevice(scopePerPlatform, device); key != "" {
		r.claim(scopePerPlatform, key, name, device.ID)
	}
	if key := scopeKeyFromDevice(scopePerUser, device); key != "" {
		r.claim(scopePerUser, key, name, device.ID)
	}
}

// IsTaken checks if a name is already in use.
func (r *NameRegistry) IsTaken(scope duplicateScope, key, name, deviceID string) bool {
	if r == nil {
		return false
	}
	normalised := normaliseName(name)
	if normalised == "" {
		return false
	}
	bucket, _ := r.bucket(scope, key, false)
	if bucket == nil {
		return false
	}
	owner := bucket[normalised]
	return owner != "" && owner != deviceID
}

func (r *NameRegistry) Update(ctx *DeviceContext, name string) {
	if r == nil || ctx == nil || ctx.Device == nil {
		return
	}
	normalised := normaliseName(name)
	if normalised == "" {
		return
	}
	r.release(ctx.Device.ID)
	r.claim(scopeGlobal, "", normalised, ctx.Device.ID)
	if key := scopeKeyFromContext(scopePerPlatform, ctx); key != "" {
		r.claim(scopePerPlatform, key, normalised, ctx.Device.ID)
	}
	if key := scopeKeyFromContext(scopePerUser, ctx); key != "" {
		r.claim(scopePerUser, key, normalised, ctx.Device.ID)
	}
}

func (r *NameRegistry) release(deviceID string) {
	if r == nil {
		return
	}
	entries := r.claims[deviceID]
	for _, c := range entries {
		bucket, _ := r.bucket(c.scope, c.key, false)
		if bucket == nil {
			continue
		}
		if current, ok := bucket[c.name]; ok && current == deviceID {
			delete(bucket, c.name)
		}
	}
	delete(r.claims, deviceID)
}

func (r *NameRegistry) claim(scope duplicateScope, key, name, deviceID string) {
	if r == nil || name == "" || deviceID == "" {
		return
	}
	bucket, normalisedKey := r.bucket(scope, key, true)
	if bucket == nil {
		return
	}
	bucket[name] = deviceID
	r.claims[deviceID] = append(r.claims[deviceID], claimRecord{scope: scope, key: normalisedKey, name: name})
}

func (r *NameRegistry) bucket(scope duplicateScope, key string, create bool) (map[string]string, string) {
	normalisedKey := normaliseScopeKey(key)
	switch scope {
	case scopePerPlatform:
		if r.perPlatform == nil {
			if !create {
				return nil, normalisedKey
			}
			r.perPlatform = make(map[string]map[string]string)
		}
		bucket := r.perPlatform[normalisedKey]
		if bucket == nil && create {
			bucket = make(map[string]string)
			r.perPlatform[normalisedKey] = bucket
		}
		return bucket, normalisedKey
	case scopePerUser:
		if r.perUser == nil {
			if !create {
				return nil, normalisedKey
			}
			r.perUser = make(map[string]map[string]string)
		}
		bucket := r.perUser[normalisedKey]
		if bucket == nil && create {
			bucket = make(map[string]string)
			r.perUser[normalisedKey] = bucket
		}
		return bucket, normalisedKey
	default:
		if r.global == nil {
			if !create {
				return nil, normalisedKey
			}
			r.global = make(map[string]string)
		}
		return r.global, normalisedKey
	}
}

func normaliseScopeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func normaliseName(name string) string {
	return strings.ToUpper(strings.TrimSpace(name))
}

func scopeKeyFromDevice(scope duplicateScope, device *intune.ManagedDevice) string {
	if device == nil {
		return ""
	}
	switch scope {
	case scopePerPlatform:
		return normaliseScopeKey(normalisePlatform(device.OperatingSystem))
	case scopePerUser:
		if id := normaliseScopeKey(device.UserID); id != "" {
			return id
		}
		if upn := normaliseScopeKey(device.UserPrincipalName); upn != "" {
			return upn
		}
	default:
		return ""
	}
	return ""
}

func scopeKeyFromContext(scope duplicateScope, ctx *DeviceContext) string {
	if ctx == nil {
		return ""
	}
	switch scope {
	case scopePerPlatform:
		if platform := ctx.AttrValue("platform"); platform != "" {
			return normaliseScopeKey(platform)
		}
		if ctx.Device != nil {
			return scopeKeyFromDevice(scopePerPlatform, ctx.Device)
		}
	case scopePerUser:
		candidates := []string{
			ctx.AttrValue("primaryUserId"),
			ctx.AttrValue("userId"),
			ctx.AttrValue("primaryUserPrincipalName"),
			ctx.AttrValue("userPrincipalName"),
			ctx.AttrValue("username"),
		}
		for _, candidate := range candidates {
			if normalised := normaliseScopeKey(candidate); normalised != "" {
				return normalised
			}
		}
		if ctx.Device != nil {
			return scopeKeyFromDevice(scopePerUser, ctx.Device)
		}
	default:
		return ""
	}
	return ""
}
