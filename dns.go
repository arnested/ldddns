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

		log.Logf(log.PriDebug, "Attempting to add address for hostname %s (IP: %s) to Avahi entry group", hostname, ipNumber)
		err := entryGroup.AddAddress(iface, avahi.ProtoInet, uint32(net.FlagMulticast), hostname, ipNumber)
		if err != nil {
			log.Logf(log.PriErr, "Failed to add address for hostname %s (IP: %s): %v", hostname, ipNumber, err)
			continue
		}

		log.Logf(log.PriDebug, "Successfully added address for hostname %s pointing to %s", hostname, ipNumber)
	}
}

func addServices(entryGroup *avahi.EntryGroup, hostname string, ips []string, services map[string]uint16, name string) {
	for _, ip := range ips {
		if ip == "" {
			continue
		}

		for service, portNumber := range services {
			log.Logf(log.PriDebug, "Attempting to add service %s for %s on hostname %s (port: %d, IP: %s) to Avahi entry group", service, name, hostname, portNumber, ip)
			err := entryGroup.AddService(
				iface,
				avahi.ProtoInet,
				0,
				name,      // Name of the service (e.g., container name)
				service,   // Type of the service (e.g., _http._tcp)
				tld,       // Domain (e.g., local)
				hostname,  // Hostname where the service is running
				portNumber, // Port number of the service
				nil,       // TXT records
			)
			if err != nil {
				log.Logf(log.PriErr, "Failed to add service %s for %s on hostname %s (port: %d, IP: %s): %v", service, name, hostname, portNumber, ip, err)
				continue
			}

			log.Logf(log.PriDebug, "Successfully added service %s for %s pointing to hostname %s (port: %d, IP: %s)", service, name, hostname, portNumber, ip)
		}
	}
}
