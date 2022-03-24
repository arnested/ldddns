package main

import (
	"net"

	"github.com/holoplot/go-avahi"
	"ldddns.arnested.dk/internal/log"
)

const (
	iface = int32(net.FlagUp)
	tld   = "local"
)

func addAddress(entryGroup *avahi.EntryGroup, hostname string, ipNumbers []string) {
	for _, ipNumber := range ipNumbers {
		if ipNumber == "" {
			continue
		}

		err := entryGroup.AddAddress(iface, avahi.ProtoInet, uint32(net.FlagMulticast), hostname, ipNumber)
		if err != nil {
			log.Logf(log.PriErr, "addAddess() failed: %v", err)

			continue
		}

		log.Logf(log.PriDebug, "added address for %q pointing to %q", hostname, ipNumber)
	}
}

func addServices(entryGroup *avahi.EntryGroup, hostname string, ips []string, services map[string]uint16, name string) {
	for _, ip := range ips {
		if ip == "" {
			continue
		}

		for service, portNumber := range services {
			err := entryGroup.AddService(
				iface,
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
				log.Logf(log.PriErr, "AddService() failed: %v", err)

				continue
			}

			log.Logf(log.PriDebug, "added service %q pointing to %q", service, hostname)
		}
	}
}
