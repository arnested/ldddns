package log

import (
	"fmt"

	"github.com/coreos/go-systemd/journal"
)

// Priority is the log level.
type Priority int

const (
	// PriEmerg is the emergency log level.
	PriEmerg Priority = iota
	// PriAlert is the alert log level.
	PriAlert
	// PerCrit is the critical log level.
	PriCrit
	// PerErr is the error log level.
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
func Logf(priority Priority, format string, a ...interface{}) {
	err := journal.Print(journal.Priority(priority), format, a...)
	if err != nil {
		panic(fmt.Errorf("could not log: %w", err))
	}
}
