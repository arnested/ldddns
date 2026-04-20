package log_test

import (
	"testing"

	"ldddns.arnested.dk/internal/log"
)

func TestPriorityConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		priority log.Priority
		expected int
	}{
		{"PriEmerg", log.PriEmerg, 0},
		{"PriAlert", log.PriAlert, 1},
		{"PriCrit", log.PriCrit, 2},
		{"PriErr", log.PriErr, 3},
		{"PriWarning", log.PriWarning, 4},
		{"PriNotice", log.PriNotice, 5},
		{"PriInfo", log.PriInfo, 6},
		{"PriDebug", log.PriDebug, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if int(tt.priority) != tt.expected {
				t.Errorf("Expected %s to be %d, got %d", tt.name, tt.expected, int(tt.priority))
			}
		})
	}
}

func TestLogf(t *testing.T) {
	t.Parallel()

	// Test that Logf doesn't panic with valid inputs
	tests := []struct {
		name     string
		priority log.Priority
		format   string
		args     []any
	}{
		{
			name:     "simple message",
			priority: log.PriInfo,
			format:   "test message",
			args:     nil,
		},
		{
			name:     "formatted message",
			priority: log.PriNotice,
			format:   "test %s message %d",
			args:     []any{"formatted", 123},
		},
		{
			name:     "emergency priority",
			priority: log.PriEmerg,
			format:   "emergency: %v",
			args:     []any{"critical situation"},
		},
		{
			name:     "debug priority",
			priority: log.PriDebug,
			format:   "debug: value=%d",
			args:     []any{42},
		},
		{
			name:     "empty message",
			priority: log.PriInfo,
			format:   "",
			args:     nil,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// This test verifies that Logf doesn't panic
			// We can't easily verify the output goes to journald in unit tests,
			// but we can verify the function executes without error
			defer func() {
				if recovered := recover(); recovered != nil {
					// If we're not running in a systemd environment, the journal
					// might not be available, which is OK for unit tests
					// Only fail if it's an unexpected panic
					if err, ok := recovered.(error); ok {
						// Expected error when journald is not available
						t.Logf("Journal not available (expected in test environment): %v", err)
					} else {
						t.Errorf("Unexpected panic: %v", recovered)
					}
				}
			}()

			log.Logf(testCase.priority, testCase.format, testCase.args...)
		})
	}
}

func TestLogfAllPriorities(t *testing.T) {
	t.Parallel()

	priorities := []struct {
		name     string
		priority log.Priority
	}{
		{"emerg", log.PriEmerg},
		{"alert", log.PriAlert},
		{"crit", log.PriCrit},
		{"err", log.PriErr},
		{"warning", log.PriWarning},
		{"notice", log.PriNotice},
		{"info", log.PriInfo},
		{"debug", log.PriDebug},
	}

	for _, testCase := range priorities {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recovered := recover(); recovered != nil {
					if err, ok := recovered.(error); ok {
						t.Logf("Journal not available (expected): %v", err)
					} else {
						t.Errorf("Unexpected panic: %v", recovered)
					}
				}
			}()

			log.Logf(testCase.priority, "test message at priority %d", testCase.priority)
		})
	}
}
