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

	e.mutex.Lock()

	if _, ok := e.groups[containerID]; !ok {
		entryGroup, err := e.avahiServer.EntryGroupNew()
		if err != nil {
			e.mutex.Unlock()

			return nil, func() {}, fmt.Errorf("error creating new entry group: %w", err)
		}

		e.groups[containerID] = entryGroup
	}

	return e.groups[containerID], commit, nil
}
