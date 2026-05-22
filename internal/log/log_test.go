package log_test

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"ldddns.arnested.dk/internal/log"
	"ldddns.arnested.dk/internal/log/logtest"
)

func TestMain(m *testing.M) {
	logtest.SilenceAll()
	os.Exit(m.Run())
}

type entry struct {
	priority log.Priority
	message  string
}

// installMock swaps log.Output for a capturing implementation that records
// each entry and returns no error. The original is restored when the test
// ends.
func installMock(t *testing.T) *[]entry {
	t.Helper()

	var captured []entry

	orig := log.Output
	log.Output = func(priority log.Priority, format string, a ...any) error {
		captured = append(captured, entry{priority, fmt.Sprintf(format, a...)})

		return nil
	}

	t.Cleanup(func() { log.Output = orig })

	return &captured
}

// assertOne fails the test unless captured contains exactly one entry with
// the given priority and message.
func assertOne(t *testing.T, captured *[]entry, priority log.Priority, message string) {
	t.Helper()

	if len(*captured) != 1 {
		t.Fatalf("expected 1 captured entry, got %d", len(*captured))
	}

	got := (*captured)[0]
	if got.priority != priority {
		t.Errorf("priority: got %d, want %d", got.priority, priority)
	}

	if got.message != message {
		t.Errorf("message: got %q, want %q", got.message, message)
	}
}

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

// TestLogf and TestLogfAllPriorities mutate the package-level log.Output and
// therefore cannot run in parallel with each other or with themselves.
//
//nolint:paralleltest // mutates shared log.Output.
func TestLogf(t *testing.T) {
	tests := []struct {
		name     string
		priority log.Priority
		format   string
		args     []any
		want     string
	}{
		{"simple message", log.PriInfo, "test message", nil, "test message"},
		{"formatted message", log.PriNotice, "test %s message %d", []any{"formatted", 123}, "test formatted message 123"},
		{"emergency priority", log.PriEmerg, "emergency: %v", []any{"critical situation"}, "emergency: critical situation"},
		{"debug priority", log.PriDebug, "debug: value=%d", []any{42}, "debug: value=42"},
		{"empty message", log.PriInfo, "", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captured := installMock(t)

			log.Logf(tc.priority, tc.format, tc.args...)
			assertOne(t, captured, tc.priority, tc.want)
		})
	}
}

//nolint:paralleltest // mutates shared log.Output.
func TestLogfAllPriorities(t *testing.T) {
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

	for _, tc := range priorities {
		t.Run(tc.name, func(t *testing.T) {
			captured := installMock(t)

			log.Logf(tc.priority, "test message at priority %d", tc.priority)
			assertOne(t, captured, tc.priority, fmt.Sprintf("test message at priority %d", tc.priority))
		})
	}
}

var errBackendUnavailable = errors.New("backend unavailable")

//nolint:paralleltest // mutates shared log.Output.
func TestLogfPanicsOnError(t *testing.T) {
	orig := log.Output
	log.Output = func(_ log.Priority, _ string, _ ...any) error {
		return errBackendUnavailable
	}

	t.Cleanup(func() { log.Output = orig })

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic when Output returns an error")
		}

		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("expected panic value to be an error, got %T", recovered)
		}

		if !errors.Is(err, errBackendUnavailable) {
			t.Errorf("unexpected error: %v", err)
		}
	}()

	log.Logf(log.PriErr, "boom")
}
