package naming

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/woodleighschool/onomazo/internal/intune"
)

// DeviceContext provides device attributes for template rendering and matching.
type DeviceContext struct {
	Device          *intune.ManagedDevice
	attrs           map[string]string
	appliedOverlays []string
	groupMembership map[string]struct{}
}

func newDeviceContext(device *intune.ManagedDevice, user *intune.UserProfile) *DeviceContext {
	ctx := &DeviceContext{
		Device:          device,
		attrs:           make(map[string]string),
		groupMembership: make(map[string]struct{}),
	}
	ctx.populateBaseAttributes()
	ctx.applyUserProfile(user)
	return ctx
}

func (c *DeviceContext) populateBaseAttributes() {
	d := c.Device
	set := c.setAttr
	set("id", d.ID)
	set("deviceId", d.ID)
	set("deviceName", d.DeviceName)
	set("currentName", d.DeviceName)
	set("serialNumber", d.SerialNumber)
	set("serial", d.SerialNumber)
	set("operatingSystem", d.OperatingSystem)
	set("os", d.OperatingSystem)
	set("osVersion", d.OSVersion)
	set("platform", normalisePlatform(d.OperatingSystem))
	set("userId", d.UserID)
	set("userDisplayName", d.UserDisplayName)
	set("userPrincipalName", d.UserPrincipalName)
	set("username", usernameFromUPN(d.UserPrincipalName))
	set("userAlias", usernameFromUPN(d.UserPrincipalName))
	first, last := splitName(d.UserDisplayName)
	set("firstName", first)
	set("lastName", last)
	set("managementAgent", d.ManagementAgent)
	set("managementState", d.ManagementState)
	set("complianceState", d.ComplianceState)
	set("deviceCategory", d.DeviceCategoryDisplay)
	set("enrollmentProfileName", d.EnrollmentProfileName)
	set("manufacturer", d.Manufacturer)
	set("model", d.Model)
	set("azureAdDeviceId", d.AzureADDeviceID)
	set("imei", d.IMEI)
	set("wifiMac", d.WiFiMacAddress)
	set("phoneNumber", d.PhoneNumber)
	set("easDeviceId", d.EasDeviceID)
	if !d.LastSyncDateTime.IsZero() {
		set("lastSync", d.LastSyncDateTime.UTC().Format(time.RFC3339))
	}
	if !d.EnrolledDateTime.IsZero() {
		set("enrolledDate", d.EnrolledDateTime.UTC().Format(time.RFC3339))
	}
}

func (c *DeviceContext) applyUserProfile(profile *intune.UserProfile) {
	if profile == nil || profile.User == nil {
		return
	}
	u := profile.User
	set := c.setAttr
	set("primaryUserId", u.ID)
	set("primaryUserDisplayName", u.DisplayName)
	set("primaryUserPrincipalName", u.UserPrincipalName)
	set("primaryUserMailNickname", u.MailNickname)
	set("primaryUserDepartment", u.Department)
	if u.Department != "" {
		set("department", u.Department)
	}
	if u.MailNickname != "" {
		set("mailNickname", u.MailNickname)
		set("username", strings.ToLower(u.MailNickname))
		set("userAlias", strings.ToLower(u.MailNickname))
	}
	if u.UserPrincipalName != "" {
		set("userPrincipalName", u.UserPrincipalName)
	}
	var groupIDs []string
	for _, g := range profile.Groups {
		if g.ID != "" {
			groupIDs = append(groupIDs, g.ID)
			c.groupMembership[strings.ToLower(g.ID)] = struct{}{}
		}
	}
	if len(groupIDs) > 0 {
		set("memberOfIds", strings.Join(groupIDs, ";"))
	}
}

func (c *DeviceContext) setAttr(key, value string) {
	key = normaliseAttrKey(key)
	if key == "" {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	c.attrs[key] = value
}

// Attr returns an attribute value or an error if missing.
func (c *DeviceContext) Attr(key string) (string, error) {
	key = normaliseAttrKey(key)
	if key == "" {
		return "", missingAttrError{}
	}
	if value := c.attrs[key]; value != "" {
		return value, nil
	}
	return "", missingAttrError{key: key}
}

// AttrValue returns an attribute value or empty string if missing.
func (c *DeviceContext) AttrValue(key string) string {
	return c.attrs[normaliseAttrKey(key)]
}

func (c *DeviceContext) HasAttr(key string) bool {
	return c.AttrValue(key) != ""
}

func (c *DeviceContext) Attributes() map[string]string {
	return c.attrs
}

func (c *DeviceContext) markOverlay(name string) {
	if name == "" {
		return
	}
	c.appliedOverlays = append(c.appliedOverlays, name)
}

// AppliedOverlays returns names of applied metadata overlays.
func (c *DeviceContext) AppliedOverlays() []string {
	cloned := make([]string, len(c.appliedOverlays))
	copy(cloned, c.appliedOverlays)
	return cloned
}

func (c *DeviceContext) hasGroup(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return false
	}
	_, ok := c.groupMembership[id]
	return ok
}

func normaliseAttrKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

type missingAttrError struct {
	key string
}

func (e missingAttrError) Error() string {
	if e.key == "" {
		return "attribute value is required"
	}
	return fmt.Sprintf("attribute %q is not available", e.key)
}

func (e missingAttrError) Attribute() string {
	return e.key
}

func normalisePlatform(os string) string {
	os = strings.ToLower(strings.TrimSpace(os))
	switch {
	case strings.HasPrefix(os, "windows"):
		return "windows"
	case strings.Contains(os, "ios"):
		return "ios"
	case strings.Contains(os, "mac"):
		return "macos"
	case strings.Contains(os, "android"):
		return "android"
	default:
		return os
	}
}

func usernameFromUPN(upn string) string {
	upn = strings.TrimSpace(upn)
	if upn == "" {
		return ""
	}
	parts := strings.Split(upn, "@")
	return parts[0]
}

func splitName(display string) (string, string) {
	display = strings.TrimSpace(display)
	if display == "" {
		return "", ""
	}
	parts := strings.Fields(display)
	if len(parts) == 1 {
		return properCase(parts[0]), ""
	}
	return properCase(parts[0]), properCase(parts[len(parts)-1])
}

func properCase(input string) string {
	if input == "" {
		return ""
	}
	runes := []rune(strings.ToLower(input))
	for i, r := range runes {
		if unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(runes)
}
