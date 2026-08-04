package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/woodleighschool/onomazo/internal/domain"
	"github.com/woodleighschool/onomazo/internal/planner"
	"github.com/woodleighschool/onomazo/internal/state"
)

type ledger interface {
	Prepare(state.Key, string, string, string, time.Time, time.Duration, int) (state.Decision, error)
	MarkFailed(state.Key, string, string, string) error
	MarkRetrying(state.Key, string, string, string) error
	MarkSubmitted(state.Key, string, string) error
	Observe(state.Key, string, string) (bool, error)
}

type cachedDevice struct {
	device      domain.Device
	refreshedAt time.Time
}

type cachedUser struct {
	user        domain.User
	refreshedAt time.Time
}

// Service reconciles complete provider snapshots through one deterministic global plan.
type Service struct {
	sources           []DeviceSource
	sourcesByName     map[string]DeviceSource
	identity          IdentityResolver
	groupAliases      map[string][]string
	planner           namePlanner
	ledger            ledger
	deviceDetailsTTL  time.Duration
	identityTTL       time.Duration
	renameRetryAfter  time.Duration
	renameMaxAttempts int
	concurrency       int
	now               func() time.Time

	devices map[state.Key]cachedDevice
	users   map[string]cachedUser
	closer  io.Closer
}

// Close releases process-level resources such as the file-state writer lock.
func (s *Service) Close() error {
	if s == nil || s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

// New creates a reconciliation service.
func New(options Options) (*Service, error) {
	if len(options.Sources) == 0 {
		return nil, fmt.Errorf("at least one device source is required")
	}
	if options.Planner == nil || options.Ledger == nil {
		return nil, fmt.Errorf("planner and state ledger are required")
	}
	if options.DeviceDetailsTTL <= 0 || options.IdentityTTL <= 0 || options.RenameRetryAfter <= 0 {
		return nil, fmt.Errorf("cache and retry durations must be greater than zero")
	}
	if options.RenameMaxAttempts <= 0 || options.Concurrency <= 0 {
		return nil, fmt.Errorf("rename attempts and concurrency must be greater than zero")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	sourcesByName := make(map[string]DeviceSource, len(options.Sources))
	for _, source := range options.Sources {
		if source == nil || source.Name() == "" {
			return nil, fmt.Errorf("device sources must have names")
		}
		if _, exists := sourcesByName[source.Name()]; exists {
			return nil, fmt.Errorf("device source %q is duplicated", source.Name())
		}
		sourcesByName[source.Name()] = source
	}
	return &Service{
		sources:           append([]DeviceSource(nil), options.Sources...),
		sourcesByName:     sourcesByName,
		identity:          options.Identity,
		groupAliases:      cloneGroups(options.GroupAliases),
		planner:           options.Planner,
		ledger:            options.Ledger,
		deviceDetailsTTL:  options.DeviceDetailsTTL,
		identityTTL:       options.IdentityTTL,
		renameRetryAfter:  options.RenameRetryAfter,
		renameMaxAttempts: options.RenameMaxAttempts,
		concurrency:       options.Concurrency,
		now:               options.Now,
		devices:           make(map[state.Key]cachedDevice),
		users:             make(map[string]cachedUser),
	}, nil
}

// Reconcile fetches a complete snapshot, produces one plan, and optionally submits allowed renames.
func (s *Service) Reconcile(ctx context.Context, apply bool) ([]Result, error) {
	now := s.now().UTC()
	fresh, err := s.listDevices(ctx)
	if err != nil {
		return nil, err
	}
	devices := s.refreshDevices(fresh, now)
	users, err := s.resolveUsers(ctx, devices, now)
	if err != nil {
		return nil, err
	}
	records := make([]planner.Record, len(devices))
	devicesByKey := make(map[state.Key]domain.Device, len(devices))
	for index, device := range devices {
		records[index] = planner.Record{Device: device, User: users[device.UserID]}
		devicesByKey[deviceKey(device)] = device
	}
	items, err := s.planner.Plan(records)
	if err != nil {
		return nil, fmt.Errorf("plan device names: %w", err)
	}
	results := make([]Result, len(items))
	for index, item := range items {
		results[index].Item = item
		if !apply && item.Status == planner.StatusRename {
			results[index].Action = ActionPlanned
		}
	}
	if !apply {
		return results, nil
	}
	return s.apply(ctx, results, devicesByKey, now)
}

func (s *Service) listDevices(ctx context.Context) ([]domain.Device, error) {
	type sourceResult struct {
		name    string
		devices []domain.Device
		err     error
	}
	resultChannel := make(chan sourceResult, len(s.sources))
	for _, source := range s.sources {
		go func() {
			devices, err := source.ListDevices(ctx)
			resultChannel <- sourceResult{name: source.Name(), devices: devices, err: err}
		}()
	}
	bySource := make(map[string][]domain.Device, len(s.sources))
	for range s.sources {
		result := <-resultChannel
		if result.err != nil {
			return nil, fmt.Errorf("list %s devices: %w", result.name, result.err)
		}
		for index := range result.devices {
			device := &result.devices[index]
			if device.ID == "" {
				return nil, fmt.Errorf("list %s devices: device ID is required", result.name)
			}
			if device.Namespace == "" {
				return nil, fmt.Errorf("list %s devices: device namespace is required", result.name)
			}
			device.Source = result.name
		}
		bySource[result.name] = result.devices
	}
	var devices []domain.Device
	for _, source := range s.sources {
		devices = append(devices, bySource[source.Name()]...)
	}
	return devices, nil
}

func (s *Service) refreshDevices(fresh []domain.Device, now time.Time) []domain.Device {
	active := make(map[state.Key]struct{}, len(fresh))
	devices := make([]domain.Device, 0, len(fresh))
	for _, incoming := range fresh {
		key := deviceKey(incoming)
		active[key] = struct{}{}
		cached, found := s.devices[key]
		identityChanged := found && (cached.device.SerialNumber != incoming.SerialNumber ||
			cached.device.UserID != incoming.UserID ||
			cached.device.Platform != incoming.Platform)
		if !found || identityChanged || !now.Before(cached.refreshedAt.Add(s.deviceDetailsTTL)) {
			cached = cachedDevice{device: incoming, refreshedAt: now}
		} else {
			cached.device.CurrentName = incoming.CurrentName
			cached.device.LastSeenAt = incoming.LastSeenAt
		}
		s.devices[key] = cached
		devices = append(devices, cached.device)
	}
	for key := range s.devices {
		if _, exists := active[key]; !exists {
			delete(s.devices, key)
		}
	}
	return devices
}

func (s *Service) resolveUsers(
	ctx context.Context,
	devices []domain.Device,
	now time.Time,
) (map[string]domain.User, error) {
	active := make(map[string]struct{})
	var stale []string
	for _, device := range devices {
		if device.UserID == "" {
			continue
		}
		active[device.UserID] = struct{}{}
		cached, found := s.users[device.UserID]
		if !found || !now.Before(cached.refreshedAt.Add(s.identityTTL)) {
			stale = append(stale, device.UserID)
		}
	}
	if s.identity != nil && len(stale) != 0 {
		resolved, err := s.identity.ResolveUsers(ctx, stale, s.groupAliases, s.concurrency)
		if err != nil {
			return nil, fmt.Errorf("resolve device users: %w", err)
		}
		for _, identifier := range stale {
			s.users[identifier] = cachedUser{user: resolved[identifier], refreshedAt: now}
		}
	}
	for identifier := range s.users {
		if _, exists := active[identifier]; !exists {
			delete(s.users, identifier)
		}
	}
	users := make(map[string]domain.User, len(active))
	for identifier := range active {
		users[identifier] = s.users[identifier].user
	}
	return users, nil
}

func (s *Service) apply(
	ctx context.Context,
	results []Result,
	devices map[state.Key]domain.Device,
	now time.Time,
) ([]Result, error) {
	type job struct {
		index    int
		decision state.Decision
		device   domain.Device
	}
	var jobs []job
	for index := range results {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result := &results[index]
		key := itemKey(result.Item)
		stateSerial := stateSerialNumber(result.ID, result.SerialNumber)
		if result.Status != planner.StatusRename {
			if _, err := s.ledger.Observe(key, stateSerial, result.CurrentName); err != nil {
				return nil, fmt.Errorf(
					"observe rename state for %s/%s/%s: %w",
					result.Source,
					result.Namespace,
					result.ID,
					err,
				)
			}
			continue
		}
		decision, err := s.ledger.Prepare(
			key,
			stateSerial,
			result.CurrentName,
			result.DesiredName,
			now,
			s.renameRetryAfter,
			s.renameMaxAttempts,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"prepare rename for %s/%s/%s: %w",
				result.Source,
				result.Namespace,
				result.ID,
				err,
			)
		}
		result.Attempts = decision.Attempts
		result.RetryAt = decision.RetryAt
		switch decision.Disposition {
		case state.DispositionFailed:
			result.Action = ActionFailed
			result.Error = decision.Failure
		case state.DispositionPending:
			result.Action = ActionPending
		case state.DispositionSubmit:
			jobs = append(jobs, job{index: index, decision: decision, device: devices[key]})
		case state.DispositionObserved:
			result.Action = ""
		default:
			return nil, fmt.Errorf(
				"prepare rename for %s/%s/%s: unknown disposition %q",
				result.Source,
				result.Namespace,
				result.ID,
				decision.Disposition,
			)
		}
	}

	jobChannel := make(chan job, len(jobs))
	for _, item := range jobs {
		jobChannel <- item
	}
	close(jobChannel)
	workerCount := min(s.concurrency, len(jobs))
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer waitGroup.Done()
			for item := range jobChannel {
				if err := ctx.Err(); err != nil {
					result := &results[item.index]
					result.Action = ActionPending
					result.Error = err.Error()
					result.RetryAt = item.decision.RetryAt
					continue
				}
				s.submitRename(ctx, &results[item.index], item.device, item.decision)
			}
		}()
	}
	waitGroup.Wait()

	var renameErrors []error
	for _, result := range results {
		if result.Error != "" {
			renameErrors = append(renameErrors, fmt.Errorf(
				"rename %s/%s/%s: %s",
				result.Source,
				result.Namespace,
				result.ID,
				result.Error,
			))
		}
	}
	return results, errors.Join(renameErrors...)
}

func (s *Service) submitRename(
	ctx context.Context,
	result *Result,
	device domain.Device,
	decision state.Decision,
) {
	source := s.sourcesByName[result.Source]
	key := itemKey(result.Item)
	stateSerial := stateSerialNumber(result.ID, result.SerialNumber)
	err := source.Rename(ctx, device, result.DesiredName)
	if err == nil {
		if stateErr := s.ledger.MarkSubmitted(key, stateSerial, result.DesiredName); stateErr != nil {
			result.Action = ActionSubmitted
			result.Error = fmt.Errorf("record submitted rename: %w", stateErr).Error()
			return
		}
		result.Action = ActionSubmitted
		result.RetryAt = time.Time{}
		return
	}
	result.Error = err.Error()
	result.Action = ActionPending
	if isPermanentRenameError(source, err) {
		if stateErr := s.ledger.MarkFailed(key, stateSerial, result.DesiredName, err.Error()); stateErr != nil {
			result.Error = errors.Join(err, stateErr).Error()
			return
		}
		result.Action = ActionFailed
		result.RetryAt = time.Time{}
		return
	}
	if stateErr := s.ledger.MarkRetrying(key, stateSerial, result.DesiredName, err.Error()); stateErr != nil {
		result.Error = errors.Join(err, stateErr).Error()
	}
	result.RetryAt = decision.RetryAt
}

func stateSerialNumber(deviceID, serialNumber string) string {
	if serialNumber != "" {
		return serialNumber
	}
	return "provider-id:" + deviceID
}

func deviceKey(device domain.Device) state.Key {
	return state.Key{Source: device.Source, Namespace: device.Namespace, DeviceID: device.ID}
}

func itemKey(item planner.Item) state.Key {
	return state.Key{Source: item.Source, Namespace: item.Namespace, DeviceID: item.ID}
}

func isPermanentRenameError(source DeviceSource, err error) bool {
	status, ok := source.StatusCode(err)
	if !ok {
		return false
	}
	switch status {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusGone,
		http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func cloneGroups(groups map[string][]string) map[string][]string {
	result := make(map[string][]string, len(groups))
	for alias, identifiers := range groups {
		result[alias] = append([]string(nil), identifiers...)
		sort.Strings(result[alias])
	}
	return result
}
