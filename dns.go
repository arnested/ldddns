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

func addAddress(eg *avahi.EntryGroup, hostname string, ips []string) {
	for _, ip := range ips {
		if ip == "" {
			continue
		}

		err := eg.AddAddress(iface, avahi.ProtoInet, 16, hostname, ip)
		if err != nil {
			log.Logf(log.PriErr, "addAddess() failed: %v", err)

			continue
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
