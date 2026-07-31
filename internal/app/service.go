package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/intune"
	"github.com/woodleighschool/onomazo/internal/naming"
)

// Service manages device naming operations.
type Service struct {
	cfg       *config.Config
	opts      *Options
	logger    *slog.Logger
	engine    *naming.Engine
	client    *intune.Client
	userCache map[string]*intune.UserProfile
}

func New(cfg *config.Config, opts *Options, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if opts == nil {
		return nil, fmt.Errorf("service options cannot be nil")
	}
	engine, err := naming.NewEngine(cfg, opts.MaxDeviceNameLen, logger)
	if err != nil {
		return nil, err
	}
	creds := opts.Credentials
	creds.GraphBaseURL = opts.GraphBaseURL
	client, err := intune.NewClient(creds)
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:       cfg,
		opts:      opts,
		logger:    logger,
		engine:    engine,
		client:    client,
		userCache: make(map[string]*intune.UserProfile),
	}, nil
}

// Run starts the continuous sync loop.
func (s *Service) Run(ctx context.Context) error {
	s.logger.Info("starting Intune renamer", "interval", s.opts.PollInterval.String(), "dryRun", s.opts.DryRun)
	ticker := time.NewTicker(s.opts.PollInterval)
	defer ticker.Stop()

	if err := s.syncOnce(ctx); err != nil {
		s.logger.Error("initial sync failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err := s.syncOnce(ctx); err != nil {
				s.logger.Error("sync failed", "error", err)
			}
		}
	}
}

// RunOnce runs a single sync operation.
func (s *Service) RunOnce(ctx context.Context) error {
	return s.syncOnce(ctx)
}

func (s *Service) syncOnce(ctx context.Context) error {
	devices, err := s.client.ListManagedDevices(ctx)
	if err != nil {
		return err
	}
	s.logger.Info("fetched devices", "count", len(devices))
	registry := naming.NewNameRegistry(devices)
	var renamed, skipped, failed, staticRenamed int
	skipReasons := make(map[string]int)
	for _, device := range devices {
		profile, err := s.resolvePrimaryUser(ctx, &device)
		if err != nil {
			profile = nil
		}
		deviceLog := s.deviceLogger(&device, profile)
		if err != nil {
			deviceLog.Debug("failed to resolve primary user", "userId", device.UserID, "userPrincipalName", device.UserPrincipalName, "error", err)
		}
		decision, err := s.engine.Decide(&device, profile, registry)
		if err != nil {
			failed++
			deviceLog.Error("naming failed", "error", err)
			continue
		}
		if !decision.ShouldUpdate {
			skipped++
			reason := decision.Reason
			if strings.TrimSpace(reason) == "" {
				reason = "unspecified"
			}
			skipReasons[reason]++
			deviceLog.Debug("skip device", "reason", reason, "rule", decision.RuleName)
			continue
		}
		if s.opts.DryRun {
			renamed++
			if decision.Static {
				staticRenamed++
			}
			deviceLog.Info("dry-run rename",
				"current", decision.CurrentName,
				"desired", decision.DesiredName,
				"rule", decision.RuleName,
				"metadata", decision.MetadataTags)
			continue
		}
		if err := s.client.SetDeviceName(ctx, device.ID, decision.DesiredName); err != nil {
			failed++
			deviceLog.Error("rename failed",
				"rule", decision.RuleName,
				"error", err)
			continue
		}
		renamed++
		if decision.Static {
			staticRenamed++
		}
		deviceLog.Info("renamed device",
			"from", decision.CurrentName,
			"to", decision.DesiredName,
			"rule", decision.RuleName,
			"metadata", decision.MetadataTags)
	}
	s.logger.Info("sync summary",
		"devices", len(devices),
		"renamed", renamed,
		"static", staticRenamed,
		"skipped", skipped,
		"failed", failed,
		"skipReasons", skipReasons)
	return nil
}

func (s *Service) resolvePrimaryUser(ctx context.Context, device *intune.ManagedDevice) (*intune.UserProfile, error) {
	userID := device.UserID
	if userID == "" {
		// fallback to UPN if available
		userID = device.UserPrincipalName
	}
	if userID == "" {
		return nil, nil
	}
	if profile, ok := s.userCache[userID]; ok {
		return profile, nil
	}
	user, err := s.client.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	groups, err := s.client.GetUserGroups(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	profile := &intune.UserProfile{
		User:   user,
		Groups: groups,
	}
	s.userCache[userID] = profile
	if user.ID != "" && user.ID != userID {
		s.userCache[user.ID] = profile
	}
	return profile, nil
}

func (s *Service) deviceLogger(device *intune.ManagedDevice, profile *intune.UserProfile) *slog.Logger {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	if device == nil {
		return logger
	}
	serial := strings.ToUpper(strings.TrimSpace(device.SerialNumber))
	logFields := []interface{}{"deviceId", device.ID, "serial", serial}

	// Add primary user if available
	if profile != nil && profile.User != nil {
		logFields = append(logFields, "primaryUser", profile.User.UserPrincipalName)
	} else {
		logFields = append(logFields, "primaryUser", "")
	}

	return logger.With(logFields...)
}
