package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/woodleighschool/onomazo/internal/domain"
)

func TestListIntuneDevicesUsesSelectedFieldsAndPaging(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1.0/deviceManagement/managedDevices" {
			return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("$top") != "999" {
			t.Errorf("$top = %q, want 999", request.URL.Query().Get("$top"))
		}
		if request.URL.Query().Get("$skiptoken") == "next" {
			return jsonResponse(http.StatusOK, map[string]any{
				"value": []map[string]any{{
					"id":                "device-2",
					"deviceName":        "SECOND",
					"serialNumber":      "SERIAL-2",
					"operatingSystem":   "Windows",
					"userPrincipalName": "unit@example.com",
				}},
			}), nil
		}
		wantSelect := []string{
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
		}
		if got := strings.Split(request.URL.Query().Get("$select"), ","); !reflect.DeepEqual(got, wantSelect) {
			t.Errorf("$select = %#v, want %#v", got, wantSelect)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"@odata.nextLink": "https://graph.test/v1.0/deviceManagement/managedDevices?$skiptoken=next&$top=999",
			"value": []map[string]any{{
				"id":               "device-1",
				"deviceName":       "FIRST",
				"serialNumber":     "SERIAL-1",
				"operatingSystem":  "macOS",
				"osVersion":        "26.0",
				"userId":           "user-1",
				"model":            "Fixture Model",
				"enrolledDateTime": "2026-07-01T00:00:00Z",
				"lastSyncDateTime": "2026-07-31T00:00:00Z",
			}},
		}), nil
	})
	client := newTestClient(t, transport)

	devices, err := client.ListIntuneDevices(context.Background(), "intune")
	if err != nil {
		t.Fatalf("ListIntuneDevices() error = %v", err)
	}
	want := []domain.Device{
		{
			Source:       "intune",
			Namespace:    managedDeviceNamespace,
			ID:           "device-1",
			CurrentName:  "FIRST",
			SerialNumber: "SERIAL-1",
			Platform:     "macos",
			EnrolledAt:   time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
			UserID:       "user-1",
			Model:        "Fixture Model",
			OSVersion:    "26.0",
			LastSeenAt:   time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			Source:       "intune",
			Namespace:    managedDeviceNamespace,
			ID:           "device-2",
			CurrentName:  "SECOND",
			SerialNumber: "SERIAL-2",
			Platform:     "windows",
			UserID:       "unit@example.com",
		},
	}
	if !reflect.DeepEqual(devices, want) {
		t.Errorf("ListIntuneDevices() = %#v, want %#v", devices, want)
	}
}

func TestRenameIntuneDeviceUsesBetaAction(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.Path, "/beta/deviceManagement/managedDevices/device-1/setDeviceName"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if got, want := payload["deviceName"], "NEW-NAME"; got != want {
			t.Errorf("deviceName = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNoContent, nil), nil
	})
	client := newTestClient(t, transport)
	if err := client.RenameIntuneDevice(context.Background(), "device-1", "NEW-NAME"); err != nil {
		t.Fatalf("RenameIntuneDevice() error = %v", err)
	}
}

func TestRenameIntuneDevicePreservesGraphStatus(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{
			"error": map[string]string{"code": "BadRequest", "message": "rename rejected"},
		}), nil
	})
	client := newTestClient(t, transport)
	err := client.RenameIntuneDevice(context.Background(), "device-1", "NEW-NAME")
	if err == nil {
		t.Fatal("RenameIntuneDevice() error = nil, want Graph error")
	}
	status, ok := StatusCode(err)
	if !ok || status != http.StatusBadRequest {
		t.Errorf("StatusCode() = %d, %t, want 400, true", status, ok)
	}
}
