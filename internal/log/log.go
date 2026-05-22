package log

import (
	"fmt"

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

// Output is invoked by Logf to emit a log entry. It defaults to systemd's
// journald; tests can replace it with a capturing implementation.
//
//nolint:gochecknoglobals // intentional: a swappable sink is the point.
var Output = func(priority Priority, format string, a ...any) error {
	return journal.Print(journal.Priority(priority), format, a...)
}

// Logf formats a log entry and sends it through Output.
func Logf(priority Priority, format string, a ...any) {
	err := Output(priority, format, a...)
	if err != nil {
		panic(fmt.Errorf("could not log: %w", err))
	}
}
