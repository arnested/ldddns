package main

import (
	"fmt"

	"github.com/holoplot/go-avahi"
)

func addToDNS(eg *avahi.EntryGroup, hostname string, ips []string, services map[string]uint16) {
	if hostname == "" {
		return
	}

	for _, ip := range ips {
		if ip == "" {
			continue
		}

		err := eg.AddAddress(avahi.InterfaceUnspec, avahi.ProtoUnspec, 0, hostname, ip)
		if err != nil {
			panic(fmt.Errorf("AddAddess() failed: %w", err))
		}

		logf(PriDebug, "added address for %q pointing to %q", hostname, ip)

		for service, portNumber := range services {
			err = eg.AddService(
				avahi.InterfaceUnspec,
				avahi.ProtoUnspec,
				0,
				hostname,
				service,
				"local",
				hostname,
				portNumber,
				nil,
			)
			if err != nil {
				panic(fmt.Errorf("AddService() failed: %w", err))
			}
		}
	}
}
