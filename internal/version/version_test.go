package version

import (
	"testing"
)

func TestVersionVariables(t *testing.T) {
	// Test that version variables are set (they have default values)
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Commit == "" {
		t.Error("Commit should not be empty")
	}
	if BuildTime == "" {
		t.Error("BuildTime should not be empty")
	}

	// Test default values
	if Version != "dev" {
		t.Logf("Version is set to: %s", Version)
	}
	if Commit != "none" {
		t.Logf("Commit is set to: %s", Commit)
	}
	if BuildTime != "unknown" {
		t.Logf("BuildTime is set to: %s", BuildTime)
	}
}
