package log

import (
	"fmt"
	"os"
	"testing"

	"github.com/coreos/go-systemd/v22/journal"
)

// Priority is the log level.
type Priority int

const (
	// PriEmerg is the emergency log level.
	PriEmerg Priority = iota
	// PriAlert is the alert log level.
	PriAlert
	// PriCrit is the critical log level.
	PriCrit
	// PriErr is the error log level.
	PriErr
	// PriWarning is the warning log level.
	PriWarning
	// PriNotice is the notice log level.
	PriNotice
	// PriInfo is the info log level.
	PriInfo
	// PriDebug is the debug log level.
	PriDebug
)

// Logf formats a log entry to systemd's journald.
//
// In tests, or when journald is not available, Logf writes to stderr
// instead. The line gets a "<N>" prefix that systemd reads as the
// priority.
func Logf(priority Priority, format string, args ...any) {
	if testing.Testing() || !journal.Enabled() {
		fmt.Fprintf(os.Stderr, "<%d>%s\n", priority, fmt.Sprintf(format, args...))

		return
	}

	err := journal.Print(journal.Priority(priority), format, args...)
	if err != nil {
		panic(fmt.Errorf("could not log: %w", err))
	}
}
