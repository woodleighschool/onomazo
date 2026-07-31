package app

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/woodleighschool/onomazo/internal/config"
	"github.com/woodleighschool/onomazo/internal/domain"
	"github.com/woodleighschool/onomazo/internal/planner"
	"github.com/woodleighschool/onomazo/internal/provider/jamf"
	"github.com/woodleighschool/onomazo/internal/provider/microsoft"
	"github.com/woodleighschool/onomazo/internal/state"
)

// Build creates provider clients, state, and a service from a validated configuration.
func Build(cfg *config.Config, mode BuildMode) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if mode != BuildReadOnly && mode != BuildApply {
		return nil, fmt.Errorf("build mode %q is not supported", mode)
	}
	namePlanner, err := planner.New(cfg)
	if err != nil {
		return nil, err
	}

	microsoftClients := make(map[string]*microsoft.Client)
	jamfClients := make(map[string]*jamf.Client)
	getMicrosoft := func(name string) (*microsoft.Client, error) {
		if client := microsoftClients[name]; client != nil {
			return client, nil
		}
		connection := cfg.Connections[name]
		client, clientErr := microsoft.NewClient(microsoft.Config{
			TenantID:     connection.TenantID,
			ClientID:     connection.ClientID,
			ClientSecret: connection.ClientSecret,
			BaseURL:      connection.BaseURL,
		})
		if clientErr != nil {
			return nil, fmt.Errorf("create Microsoft connection %q: %w", name, clientErr)
		}
		microsoftClients[name] = client
		return client, nil
	}
	getJamf := func(name string) (*jamf.Client, error) {
		if client := jamfClients[name]; client != nil {
			return client, nil
		}
		connection := cfg.Connections[name]
		client, clientErr := jamf.NewClient(jamf.Config{
			URL:          connection.URL,
			ClientID:     connection.ClientID,
			ClientSecret: connection.ClientSecret,
		})
		if clientErr != nil {
			return nil, fmt.Errorf("create Jamf connection %q: %w", name, clientErr)
		}
		jamfClients[name] = client
		return client, nil
	}

	sources := make([]DeviceSource, 0, len(cfg.Devices))
	for _, source := range cfg.Devices {
		switch source.Type {
		case "intune":
			client, clientErr := getMicrosoft(source.Connection)
			if clientErr != nil {
				return nil, clientErr
			}
			sources = append(sources, &intuneSource{
				name:      source.Name,
				platforms: append([]string(nil), source.Platforms...),
				client:    client,
			})
		case "jamf":
			client, clientErr := getJamf(source.Connection)
			if clientErr != nil {
				return nil, clientErr
			}
			sources = append(sources, &jamfSource{
				name:      source.Name,
				platforms: append([]string(nil), source.Platforms...),
				client:    client,
			})
		default:
			return nil, fmt.Errorf("device source type %q is not supported", source.Type)
		}
	}

	var identity IdentityResolver
	var groupAliases map[string][]string
	if cfg.Identity != nil {
		client, clientErr := getMicrosoft(cfg.Identity.Connection)
		if clientErr != nil {
			return nil, clientErr
		}
		identity = client
		groupAliases = cfg.Identity.Groups
	}
	store, closer, err := buildStore(cfg.State, mode)
	if err != nil {
		return nil, err
	}
	intentLedger, err := state.Open(store)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("open rename state: %w", err)
	}
	service, err := New(Options{
		Sources:           sources,
		Identity:          identity,
		GroupAliases:      groupAliases,
		Planner:           namePlanner,
		Ledger:            intentLedger,
		DeviceDetailsTTL:  cfg.Reconcile.DeviceDetailsTTL.Duration,
		IdentityTTL:       cfg.Reconcile.IdentityTTL.Duration,
		RenameRetryAfter:  cfg.Reconcile.RenameRetryAfter.Duration,
		RenameMaxAttempts: cfg.Reconcile.RenameMaxAttempts,
		Concurrency:       cfg.Reconcile.Concurrency,
	})
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	service.closer = closer
	return service, nil
}

func buildStore(configState config.State, mode BuildMode) (state.Store, io.Closer, error) {
	switch configState.Type {
	case "memory":
		return state.NewMemoryStore(), nil, nil
	case "file":
		var store *state.FileStore
		var err error
		if mode == BuildApply {
			store, err = state.NewLockedFileStore(configState.Path)
		} else {
			store, err = state.NewFileStore(configState.Path)
		}
		if err != nil {
			return nil, nil, err
		}
		return store, store, nil
	default:
		return nil, nil, fmt.Errorf("state type %q is not supported", configState.Type)
	}
}

type intuneSource struct {
	name      string
	platforms []string
	client    *microsoft.Client
}

func (s *intuneSource) Name() string {
	return s.name
}

func (s *intuneSource) ListDevices(ctx context.Context) ([]domain.Device, error) {
	devices, err := s.client.ListIntuneDevices(ctx, s.name)
	if err != nil {
		return nil, err
	}
	return filterPlatforms(devices, s.platforms), nil
}

func (s *intuneSource) Rename(ctx context.Context, device domain.Device, desiredName string) error {
	return s.client.RenameIntuneDevice(ctx, device.ID, desiredName)
}

func (*intuneSource) StatusCode(err error) (int, bool) {
	return microsoft.StatusCode(err)
}

type jamfSource struct {
	name      string
	platforms []string
	client    *jamf.Client
}

func (s *jamfSource) Name() string {
	return s.name
}

func (s *jamfSource) ListDevices(ctx context.Context) ([]domain.Device, error) {
	var devices []domain.Device
	if slices.Contains(s.platforms, "macos") {
		computers, err := s.client.ListComputers(ctx, s.name)
		if err != nil {
			return nil, err
		}
		devices = append(devices, computers...)
	}
	if slices.Contains(s.platforms, "ios") {
		mobileDevices, err := s.client.ListMobileDevices(ctx, s.name)
		if err != nil {
			return nil, err
		}
		devices = append(devices, mobileDevices...)
	}
	return devices, nil
}

func (s *jamfSource) Rename(ctx context.Context, device domain.Device, desiredName string) error {
	switch device.Platform {
	case "macos":
		return s.client.RenameComputer(ctx, device.ID, desiredName)
	case "ios":
		return s.client.RenameMobileDevice(ctx, device.ID, desiredName)
	default:
		return fmt.Errorf("jamf platform %q cannot be renamed", device.Platform)
	}
}

func (*jamfSource) StatusCode(err error) (int, bool) {
	return jamf.StatusCode(err)
}

func filterPlatforms(devices []domain.Device, platforms []string) []domain.Device {
	result := make([]domain.Device, 0, len(devices))
	for _, device := range devices {
		if slices.Contains(platforms, device.Platform) {
			result = append(result, device)
		}
	}
	return result
}
