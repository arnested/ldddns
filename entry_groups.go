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
	commit := func() {
		defer e.mutex.Unlock()

		empty, err := e.groups[containerID].IsEmpty()
		if err != nil {
			log.Logf(log.PriErr, "checking whether Avahi entry group is empty: %v", err)
		}

		if !empty {
			err := e.groups[containerID].Commit()
			if err != nil {
				log.Logf(log.PriErr, "error committing: %v", err)
			}
		}
	}

	log.Logf(log.PriDebug, "about to get lock for container ID: %s", containerID)
	e.mutex.Lock()
	log.Logf(log.PriDebug, "got lock for container ID: %s", containerID)

	if _, ok := e.groups[containerID]; !ok {
		entryGroup, err := e.avahiServer.EntryGroupNew()
		if err != nil {
			e.mutex.Unlock()

			return nil, func() {}, fmt.Errorf("error creating new entry group: %w", err)
		}

		go logStateChanges(containerID, entryGroup.StateChangeChannel)

		e.groups[containerID] = entryGroup
	}

	return e.groups[containerID], commit, nil
}

// logStateChanges reads the state changes of an entry group. When one of
// these channels is full, go-avahi stops sending signals to all entry groups.
func logStateChanges(containerID string, states <-chan avahi.EntryGroupState) {
	for state := range states {
		switch state.State {
		case avahi.EntryGroupCollision:
			log.Logf(log.PriWarning, "Avahi name collision for container %s", containerID)
		case avahi.EntryGroupFailure:
			log.Logf(log.PriWarning, "Avahi registration failed for container %s: %s", containerID, state.Error)
		default:
			log.Logf(log.PriDebug, "Avahi entry group state for container %s: %d", containerID, state.State)
		}
	}
}
