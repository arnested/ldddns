package main

import (
	"testing"

	"github.com/moby/moby/api/types/container"
	internalContainer "ldddns.arnested.dk/internal/container"
)

func createTestContainer(labels map[string]string) internalContainer.Container {
	return internalContainer.Container{
		ID: "test-container",
		Config: &container.Config{
			Labels: labels,
		},
	}
}

func TestIgnoreOneoffEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		labels       map[string]string
		expectIgnore bool
	}{
		{
			name: "oneoff is True",
			labels: map[string]string{
				"com.docker.compose.oneoff": "True",
			},
			expectIgnore: true,
		},
		{
			name: "oneoff is False",
			labels: map[string]string{
				"com.docker.compose.oneoff": "False",
			},
			expectIgnore: false,
		},
		{
			name:         "oneoff label missing",
			labels:       map[string]string{},
			expectIgnore: false,
		},
		{
			name: "oneoff with unexpected value",
			labels: map[string]string{
				"com.docker.compose.oneoff": "yes",
			},
			expectIgnore: false,
		},
	}

	config := Config{IgnoreDockerComposeOneoff: true}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			containerInfo := createTestContainer(testCase.labels)
			result := ignoreOneoff(containerInfo, config)

			if result != testCase.expectIgnore {
				t.Errorf("Expected ignoreOneoff to return %v, got %v", testCase.expectIgnore, result)
			}
		})
	}
}

func TestIgnoreOneoffDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels map[string]string
	}{
		{
			name: "oneoff is True",
			labels: map[string]string{
				"com.docker.compose.oneoff": "True",
			},
		},
		{
			name: "oneoff is False",
			labels: map[string]string{
				"com.docker.compose.oneoff": "False",
			},
		},
	}

	config := Config{IgnoreDockerComposeOneoff: false}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			containerInfo := createTestContainer(testCase.labels)
			result := ignoreOneoff(containerInfo, config)

			if result {
				t.Error("Expected ignoreOneoff to return false when disabled")
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	t.Parallel()

	// Test that getVersion returns a non-empty string
	version := getVersion()
	if version == "" {
		t.Error("Expected getVersion to return non-empty string")
	}
}

func TestGops(t *testing.T) {
	t.Parallel()

	// Test that gops with start=false doesn't panic
	gops(false)

	// We can't easily test start=true without actually starting the agent
	// which could conflict with other tests or require cleanup
}

func TestNewEntryGroups(t *testing.T) {
	t.Parallel()

	// Test that newEntryGroups creates a valid entryGroups instance
	// We pass nil since we're just testing the constructor logic
	egs := newEntryGroups(nil)

	if egs == nil {
		t.Fatal("Expected newEntryGroups to return non-nil")
	}

	if egs.groups == nil {
		t.Error("Expected groups map to be initialized")
	}

	if egs.avahiServer != nil {
		t.Error("Expected avahiServer to be nil when passed nil")
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()

	// Test that constants are set correctly
	if tld != "local" {
		t.Errorf("Expected tld to be 'local', got %q", tld)
	}
}

func TestEntryGroupsWithoutGroup(t *testing.T) {
	t.Parallel()

	egs := newEntryGroups(nil)

	// get without create must not create a group, and commit must release
	// the lock.
	entryGroup, commit, err := egs.get("unknown", false)
	commit()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if entryGroup != nil {
		t.Error("Expected no entry group for an unknown container")
	}

	if len(egs.groups) != 0 {
		t.Error("Expected get without create to leave the groups map empty")
	}

	// free of an unknown container must do nothing. It must not call the
	// nil Avahi server.
	egs.free("unknown")

	err = resetContainer(egs, "unknown")
	if err != nil {
		t.Errorf("Expected no error resetting an unknown container, got %v", err)
	}
}
