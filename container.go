package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"honnef.co/go/netdb"
)

func handleContainer(
	ctx context.Context,
	container types.ContainerJSON,
	egs *EntryGroups,
	status string,
) {
	eg, commit, err := egs.Get(container.ID)
	defer commit()

	if err != nil {
		panic(fmt.Errorf("cannot get entry group for container %q: %w", container.ID, err))
	}

	empty, err := eg.IsEmpty()
	if err != nil {
		logf(PriErr, "checking whether Avahi entry group is empty: %v", err)

		return
	}

	if !empty {
		err := eg.Reset()
		if err != nil {
			logf(PriErr, "resetting Avahi entry group is empty: %v", err)

			return
		}
	}

	if status == "die" || status == "kill" || status == "pause" {
		return
	}

	ips := extractIPNumbers(ctx, container)
	if len(ips) == 0 {
		return
	}

	hostname := extractHostname(ctx, container)
	services := extractServices(ctx, container)

	if hostname != "" {
		hostname = rewriteHostname(hostname)
		addToDNS(eg, hostname, ips, services, container.Name[1:], true)
	}

	containerHostname := rewriteHostname(container.Name[1:] + ".local")
	addToDNS(eg, containerHostname, ips, services, container.Name[1:], hostname == "")
}

// extractIPnumbers from a container.
func extractIPNumbers(_ context.Context, container types.ContainerJSON) []string {
	ips := []string{}

	if container.NetworkSettings.IPAddress != "" {
		ips = append(ips, container.NetworkSettings.IPAddress)
	}

	for _, v := range container.NetworkSettings.Networks {
		ips = append(ips, v.IPAddress)
	}

	return ips
}

// extractServices from a container.
func extractServices(_ context.Context, container types.ContainerJSON) map[string]uint16 {
	services := map[string]uint16{}

	for k := range container.NetworkSettings.Ports {
		port := strings.SplitN(string(k), "/", 2)

		proto := netdb.GetProtoByName(port[1])

		portNumber, err := strconv.ParseUint(port[0], 10, 16)
		if err != nil {
			log.Printf("Could not get port number from %q", k)

			continue
		}

		service := netdb.GetServByPort(int(portNumber), proto)

		if service == nil || proto == nil {
			continue
		}

		services[fmt.Sprintf("_%s._%s", service.Name, proto.Name)] = uint16(portNumber)
	}

	return services
}

// extractHostname from a container.
func extractHostname(_ context.Context, container types.ContainerJSON) string {
	for _, v := range container.Config.Env {
		if strings.HasPrefix(v, "VIRTUAL_HOST=") {
			return v[13:]
		}
	}

	return ""
}
