package main

import (
	"fmt"
	"sync"

	"github.com/holoplot/go-avahi"
	"ldddns.arnested.dk/internal/log"
)

type entryGroups struct {
	avahiServer *avahi.Server
	groups      map[string]*avahi.EntryGroup
	mutex       sync.Mutex
}

func newEntryGroups(avahiServer *avahi.Server) *entryGroups {
	return &entryGroups{
		avahiServer: avahiServer,
		groups:      make(map[string]*avahi.EntryGroup),
		mutex:       sync.Mutex{},
	}
}

func (e *entryGroups) get(containerID string) (*avahi.EntryGroup, func(), error) {
	e.mutex.Lock()

	groupSuccessfullyRetrieved := true
	if _, ok := e.groups[containerID]; !ok {
		log.Logf(log.PriDebug, "Attempting to create new Avahi entry group for container %s", containerID)
		eg, err := e.avahiServer.EntryGroupNew()
		if err != nil {
			e.mutex.Unlock() // Unlock before returning due to error
			groupSuccessfullyRetrieved = false
			// Error is already descriptive, fmt.Errorf("error creating new entry group: %w", err)
			return nil, func() { e.mutex.Unlock() }, fmt.Errorf("error creating new Avahi entry group for container %s: %w", containerID, err)
		}
		log.Logf(log.PriDebug, "Successfully created new Avahi entry group for container %s", containerID)
		e.groups[containerID] = eg
	}

	commit := func() {
		defer e.mutex.Unlock()

		if !groupSuccessfullyRetrieved {
			return
		}

		group := e.groups[containerID]
		if group == nil { // Should not happen if groupSuccessfullyRetrieved is true
			log.Logf(log.PriCrit, "internal error: group for container %s is nil despite successful retrieval flag in commit()", containerID)
			return
		}

		log.Logf(log.PriDebug, "Checking if Avahi entry group for %s needs committing", containerID)
		empty, err := group.IsEmpty()
		if err != nil {
			log.Logf(log.PriErr, "Error checking whether Avahi entry group for %s is empty: %v", containerID, err)
			// If we can't check if it's empty, we probably shouldn't try to commit it.
			return
		}
		log.Logf(log.PriDebug, "Avahi entry group for %s is empty: %t (before commit check)", containerID, empty)

		if !empty {
			log.Logf(log.PriDebug, "Attempting to commit Avahi entry group for %s", containerID)
			err := group.Commit()
			if err != nil {
				log.Logf(log.PriErr, "Error committing Avahi entry group for %s: %v", containerID, err)
			} else {
				log.Logf(log.PriDebug, "Successfully committed Avahi entry group for %s", containerID)
			}
		} else {
			log.Logf(log.PriDebug, "Avahi entry group for %s is empty, no commit needed", containerID)
		}
	}

	return e.groups[containerID], commit, nil
}
