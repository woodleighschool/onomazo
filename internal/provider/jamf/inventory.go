package jamf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/woodleighschool/onomazo/internal/domain"
)

const jamfPageSize = 1000

const (
	// ComputerNamespace identifies Jamf's computer inventory ID space.
	ComputerNamespace = "computers"
	// MobileDeviceNamespace identifies Jamf's mobile-device ID space.
	MobileDeviceNamespace = "mobile_devices"
)

// ListComputers returns a provider-neutral snapshot from the Jamf Pro v4 computer inventory API.
func (c *Client) ListComputers(ctx context.Context, source string) ([]domain.Device, error) {
	if err := validateInventoryRequest(c, source); err != nil {
		return nil, err
	}
	var devices []domain.Device
	for page := 0; ; page++ {
		query := inventoryQuery(page, "GENERAL", "HARDWARE", "OPERATING_SYSTEM", "USER_AND_LOCATION")
		query.Set("sort", "id:asc")
		body, err := c.request(ctx, http.MethodGet, "/api/v4/computers-inventory", query, nil, http.StatusOK)
		if err != nil {
			return nil, fmt.Errorf("list Jamf computers: %w", err)
		}
		var result computerSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode Jamf computers: %w", err)
		}
		for _, computer := range result.Results {
			devices = append(devices, computer.device(source))
		}
		if len(result.Results) == 0 || len(devices) >= result.TotalCount {
			break
		}
	}
	return devices, nil
}

// ListMobileDevices returns supported iOS and iPadOS records from the Jamf Pro v2 mobile inventory API.
func (c *Client) ListMobileDevices(ctx context.Context, source string) ([]domain.Device, error) {
	if err := validateInventoryRequest(c, source); err != nil {
		return nil, err
	}
	var devices []domain.Device
	seen := 0
	for page := 0; ; page++ {
		query := inventoryQuery(page, "GENERAL", "HARDWARE", "USER_AND_LOCATION")
		query.Set("exception-handling", "LENIENT")
		query.Set("sort", "mobileDeviceId:asc")
		body, err := c.request(ctx, http.MethodGet, "/api/v2/mobile-devices/detail", query, nil, http.StatusOK)
		if err != nil {
			return nil, fmt.Errorf("list Jamf mobile devices: %w", err)
		}
		var result mobileSearchResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode Jamf mobile devices: %w", err)
		}
		seen += len(result.Results)
		for _, mobile := range result.Results {
			if !strings.EqualFold(mobile.DeviceType, "ios") {
				continue
			}
			devices = append(devices, mobile.device(source))
		}
		if len(result.Results) == 0 || seen >= result.TotalCount {
			break
		}
	}
	return devices, nil
}

// RenameComputer updates the inventory name using the Jamf Pro v4 computer detail endpoint.
func (c *Client) RenameComputer(ctx context.Context, deviceID, desiredName string) error {
	if err := validateRenameRequest(c, deviceID, desiredName); err != nil {
		return err
	}
	payload := map[string]any{"general": map[string]string{"name": desiredName}}
	_, err := c.request(
		ctx,
		http.MethodPatch,
		"/api/v4/computers-inventory-detail/"+url.PathEscape(deviceID),
		nil,
		payload,
		http.StatusNoContent,
	)
	if err != nil {
		return fmt.Errorf("rename Jamf computer: %w", err)
	}
	return nil
}

// RenameMobileDevice updates the inventory name without enabling Jamf's persistent name enforcement.
func (c *Client) RenameMobileDevice(ctx context.Context, deviceID, desiredName string) error {
	if err := validateRenameRequest(c, deviceID, desiredName); err != nil {
		return err
	}
	payload := map[string]string{"name": desiredName}
	_, err := c.request(
		ctx,
		http.MethodPatch,
		"/api/v2/mobile-devices/"+url.PathEscape(deviceID),
		nil,
		payload,
		http.StatusOK,
	)
	if err != nil {
		return fmt.Errorf("rename Jamf mobile device: %w", err)
	}
	return nil
}

type computerSearchResult struct {
	TotalCount int              `json:"totalCount"`
	Results    []computerRecord `json:"results"`
}

type computerRecord struct {
	ID      string `json:"id"`
	General struct {
		Name             string `json:"name"`
		Platform         string `json:"platform"`
		ReportDate       string `json:"reportDate"`
		LastCheckIn      string `json:"lastCheckIn"`
		LastContact      string `json:"lastContact"`
		LastEnrolledDate string `json:"lastEnrolledDate"`
	} `json:"general"`
	Hardware struct {
		Model        string `json:"model"`
		SerialNumber string `json:"serialNumber"`
	} `json:"hardware"`
	OperatingSystem struct {
		Version string `json:"version"`
	} `json:"operatingSystem"`
	UserAndLocation struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"userAndLocation"`
}

func (r computerRecord) device(source string) domain.Device {
	return domain.Device{
		Source:       source,
		Namespace:    ComputerNamespace,
		ID:           r.ID,
		CurrentName:  r.General.Name,
		SerialNumber: r.Hardware.SerialNumber,
		Platform:     "macos",
		EnrolledAt:   parseTimestamp(r.General.LastEnrolledDate),
		UserID:       firstNonEmpty(r.UserAndLocation.Email, r.UserAndLocation.Username),
		Model:        r.Hardware.Model,
		OSVersion:    r.OperatingSystem.Version,
		LastSeenAt: parseFirstTimestamp(
			r.General.LastContact,
			r.General.LastCheckIn,
			r.General.ReportDate,
		),
	}
}

type mobileSearchResult struct {
	TotalCount int            `json:"totalCount"`
	Results    []mobileRecord `json:"results"`
}

type mobileRecord struct {
	MobileDeviceID string `json:"mobileDeviceId"`
	DeviceType     string `json:"deviceType"`
	General        struct {
		DisplayName             string `json:"displayName"`
		LastInventoryUpdateDate string `json:"lastInventoryUpdateDate"`
		LastContactDate         string `json:"lastContactDate"`
		LastEnrolledDate        string `json:"lastEnrolledDate"`
		OSVersion               string `json:"osVersion"`
	} `json:"general"`
	Hardware struct {
		SerialNumber string `json:"serialNumber"`
		Model        string `json:"model"`
	} `json:"hardware"`
	UserAndLocation struct {
		Username     string `json:"username"`
		EmailAddress string `json:"emailAddress"`
	} `json:"userAndLocation"`
}

func (r mobileRecord) device(source string) domain.Device {
	return domain.Device{
		Source:       source,
		Namespace:    MobileDeviceNamespace,
		ID:           r.MobileDeviceID,
		CurrentName:  r.General.DisplayName,
		SerialNumber: r.Hardware.SerialNumber,
		Platform:     "ios",
		EnrolledAt:   parseTimestamp(r.General.LastEnrolledDate),
		UserID:       firstNonEmpty(r.UserAndLocation.EmailAddress, r.UserAndLocation.Username),
		Model:        r.Hardware.Model,
		OSVersion:    r.General.OSVersion,
		LastSeenAt: parseFirstTimestamp(
			r.General.LastContactDate,
			r.General.LastInventoryUpdateDate,
		),
	}
}

func inventoryQuery(page int, sections ...string) url.Values {
	return url.Values{
		"page":      {strconv.Itoa(page)},
		"page-size": {strconv.Itoa(jamfPageSize)},
		"section":   sections,
	}
}

func validateInventoryRequest(client *Client, source string) error {
	if client == nil || client.httpClient == nil {
		return fmt.Errorf("jamf client is required")
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("device source is required")
	}
	return nil
}

func validateRenameRequest(client *Client, deviceID, desiredName string) error {
	if client == nil || client.httpClient == nil {
		return fmt.Errorf("jamf client is required")
	}
	if strings.TrimSpace(deviceID) == "" || desiredName == "" {
		return fmt.Errorf("device ID and desired name are required")
	}
	return nil
}

func parseFirstTimestamp(values ...string) time.Time {
	for _, value := range values {
		if parsed := parseTimestamp(value); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Time{}
}

func parseTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
