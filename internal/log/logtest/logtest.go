// Package logtest provides helpers for installing test doubles for the log
// package's Output sink. Test packages whose code paths call log.Logf use
// these helpers to keep tests from touching journald.
package logtest

import (
	"ldddns.arnested.dk/internal/log"
)

// SilenceAll replaces log.Output with a no-op for the lifetime of the test
// binary. Call it from TestMain.
func SilenceAll() {
	log.Output = func(log.Priority, string, ...any) error { return nil }
}
