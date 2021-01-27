package main

import "github.com/coreos/go-systemd/journal"

type Priority int

// nolint:deadcode
const (
	PriEmerg Priority = iota
	PriAlert
	PriCrit
	PriErr
	PriWarning
	PriNotice
	PriInfo
	PriDebug
)

func logf(priority Priority, format string, a ...interface{}) {
	_ = journal.Print(journal.Priority(priority), format, a...)
}
