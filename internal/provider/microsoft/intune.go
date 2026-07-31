package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	devicemanagement "github.com/microsoftgraph/msgraph-sdk-go/devicemanagement"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	odataerrors "github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"

	"github.com/woodleighschool/onomazo/internal/domain"
)

const graphPageSize int32 = 999

// ListIntuneDevices returns a provider-neutral snapshot using one paged managedDevices query.
func (c *Client) ListIntuneDevices(ctx context.Context, source string) ([]domain.Device, error) {
	if c == nil || c.graph == nil {
		return nil, fmt.Errorf("graph client is required")
	}
	if source == "" {
		return nil, fmt.Errorf("device source is required")
	}
	page, err := c.graph.DeviceManagement().ManagedDevices().Get(
		ctx,
		&devicemanagement.ManagedDevicesRequestBuilderGetRequestConfiguration{
			QueryParameters: &devicemanagement.ManagedDevicesRequestBuilderGetQueryParameters{
				Select: []string{
					"id",
					"deviceName",
					"serialNumber",
					"operatingSystem",
					"osVersion",
					"userId",
					"userPrincipalName",
					"model",
					"enrolledDateTime",
					"lastSyncDateTime",
				},
				Top: new(graphPageSize),
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list Intune devices: %w", err)
	}
	if page == nil {
		return nil, fmt.Errorf("list Intune devices: Graph returned no response")
	}
	iterator, err := graphcore.NewPageIterator[graphmodels.ManagedDeviceable](
		page,
		c.graph.GetAdapter(),
		graphmodels.CreateManagedDeviceCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("page Intune devices: %w", err)
	}

	var devices []domain.Device
	err = iterator.Iterate(ctx, func(device graphmodels.ManagedDeviceable) bool {
		userID := dereference(device.GetUserId())
		if userID == "" {
			userID = dereference(device.GetUserPrincipalName())
		}
		converted := domain.Device{
			Source:       source,
			ID:           dereference(device.GetId()),
			CurrentName:  dereference(device.GetDeviceName()),
			SerialNumber: dereference(device.GetSerialNumber()),
			Platform:     normalizePlatform(dereference(device.GetOperatingSystem())),
			UserID:       userID,
			Model:        dereference(device.GetModel()),
			OSVersion:    dereference(device.GetOsVersion()),
		}
		if enrolledAt := device.GetEnrolledDateTime(); enrolledAt != nil {
			converted.EnrolledAt = enrolledAt.UTC()
		}
		if lastSeenAt := device.GetLastSyncDateTime(); lastSeenAt != nil {
			converted.LastSeenAt = lastSeenAt.UTC()
		}
		devices = append(devices, converted)
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("page Intune devices: %w", err)
	}
	return devices, nil
}

// RenameIntuneDevice submits the beta setDeviceName action required by Intune.
func (c *Client) RenameIntuneDevice(ctx context.Context, deviceID, desiredName string) error {
	if c == nil || c.graph == nil {
		return fmt.Errorf("graph client is required")
	}
	if deviceID == "" || desiredName == "" {
		return fmt.Errorf("device ID and desired name are required")
	}
	payload, err := json.Marshal(map[string]string{"deviceName": desiredName})
	if err != nil {
		return fmt.Errorf("encode Intune rename: %w", err)
	}
	request := abstractions.NewRequestInformationWithMethodAndUrlTemplateAndPathParameters(
		abstractions.POST,
		c.betaBaseURL+"/deviceManagement/managedDevices/{managedDeviceId}/setDeviceName",
		map[string]string{"managedDeviceId": deviceID},
	)
	request.SetStreamContentAndContentType(payload, "application/json")
	if err := c.graph.GetAdapter().SendNoContent(ctx, request, graphErrorMapping()); err != nil {
		return fmt.Errorf("rename Intune device: %w", err)
	}
	return nil
}

func graphErrorMapping() abstractions.ErrorMappings {
	return abstractions.ErrorMappings{
		"4XX": odataerrors.CreateODataErrorFromDiscriminatorValue,
		"5XX": odataerrors.CreateODataErrorFromDiscriminatorValue,
	}
}

func normalizePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "ios", "ipados":
		return "ios"
	case "macos", "mac os x", "osx":
		return "macos"
	case "windows":
		return "windows"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}
