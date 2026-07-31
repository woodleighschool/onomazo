package planner

import (
	"github.com/woodleighschool/onomazo/internal/domain"
)

// Record pairs one managed device with its resolved primary user.
type Record struct {
	Device domain.Device
	User   domain.User
}

// Status describes the complete outcome for one discovered device.
type Status string

const (
	StatusExcluded   Status = "excluded"
	StatusInvalid    Status = "invalid"
	StatusRename     Status = "rename"
	StatusUnchanged  Status = "unchanged"
	StatusUnmanaged  Status = "unmanaged"
	StatusUnresolved Status = "unresolved"
)

// Item is the deterministic plan entry emitted for one discovered device.
type Item struct {
	Source       string `json:"source"`
	Namespace    string `json:"namespace"`
	ID           string `json:"id"`
	SerialNumber string `json:"serial_number"`
	Platform     string `json:"platform"`
	CurrentName  string `json:"current_name"`
	DesiredName  string `json:"desired_name,omitempty"`
	User         string `json:"user,omitempty"`
	Rule         string `json:"rule,omitempty"`
	Status       Status `json:"status"`
	Reason       string `json:"reason,omitempty"`
}
