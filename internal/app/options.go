package app

import (
	"time"

	"github.com/woodleighschool/onomazo/internal/intune"
)

// Options configures Service behavior.
type Options struct {
	PollInterval     time.Duration
	DryRun           bool
	MaxDeviceNameLen int
	GraphBaseURL     string
	Credentials      intune.Credentials
}
