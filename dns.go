package main

import (
	"fmt"
	"net"

	"github.com/holoplot/go-avahi"
	"ldddns.arnested.dk/internal/log"
)

const tld = "local"

func addAddress(eg *avahi.EntryGroup, hostname string, ips []string) {
	for _, ip := range ips {
		if ip == "" {
			continue
		}

		err := eg.AddAddress(int32(net.FlagBroadcast), avahi.ProtoInet, 16, hostname, ip)
		if err != nil {
			panic(fmt.Errorf("AddAddess() failed: %w", err))
		}

		log.Logf(log.PriDebug, "added address for %q pointing to %q", hostname, ip)
	}
}

func addServices(eg *avahi.EntryGroup, hostname string, ips []string, services map[string]uint16, name string) {
	for _, ip := range ips {
		if ip == "" {
			continue
		}

		for service, portNumber := range services {
			err := eg.AddService(
				int32(net.FlagBroadcast),
				avahi.ProtoInet,
				0,
				name,
				service,
				tld,
				hostname,
				portNumber,
				nil,
			)
			if err != nil {
				panic(fmt.Errorf("AddService() failed: %w", err))
			}

			log.Logf(log.PriDebug, "added service %q pointing to %q", service, hostname)
		}
	}
}
