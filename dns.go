package main

import (
	"fmt"
	"net"

	"github.com/holoplot/go-avahi"
)

func addToDNS(eg *avahi.EntryGroup, hostname string, ips []string, services map[string]uint16, name string, srv bool) {
	if hostname == "" {
		return
	}

	for _, ip := range ips {
		if ip == "" {
			continue
		}

		err := eg.AddAddress(int32(net.FlagBroadcast), avahi.ProtoInet, 16, hostname, ip)
		if err != nil {
			panic(fmt.Errorf("AddAddess() failed: %w", err))
		}

		logf(PriDebug, "added address for %q pointing to %q", hostname, ip)

		if srv {
			for service, portNumber := range services {
				err = eg.AddService(
					int32(net.FlagBroadcast),
					avahi.ProtoInet,
					0,
					name,
					service,
					"local",
					hostname,
					portNumber,
					nil,
				)
				if err != nil {
					panic(fmt.Errorf("AddService() failed: %w", err))
				}

				logf(PriDebug, "added service %q pointing to %q", service, hostname)
			}
		}
	}
}
