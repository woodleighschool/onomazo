package domain

import "time"

// Device is the provider-neutral device record used by naming expressions.
type Device struct {
	Source       string            `cel:"source"        json:"source"`
	ID           string            `cel:"id"            json:"id"`
	CurrentName  string            `cel:"current_name"  json:"current_name"`
	SerialNumber string            `cel:"serial_number" json:"serial_number"`
	Platform     string            `cel:"platform"      json:"platform"`
	EnrolledAt   time.Time         `cel:"enrolled_at"   json:"enrolled_at"`
	UserID       string            `cel:"user_id"       json:"user_id"`
	Model        string            `cel:"model"         json:"model"`
	OSVersion    string            `cel:"os_version"    json:"os_version"`
	LastSeenAt   time.Time         `cel:"last_seen_at"  json:"last_seen_at"`
	Attributes   map[string]string `cel:"attributes"    json:"attributes,omitempty"`
}
