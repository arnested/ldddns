package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"honnef.co/go/netdb"
)

func handleContainer(
	ctx context.Context,
	docker *client.Client,
	containerID string,
	egs *EntryGroups,
	status string,
) error {
	eg, commit, err := egs.Get(containerID)
	defer commit()

	if err != nil {
		return fmt.Errorf("cannot get entry group for container: %w", err)
	}

	empty, err := eg.IsEmpty()
	if err != nil {
		return fmt.Errorf("checking whether Avahi entry group is empty: %w", err)
	}

	if !empty {
		err := eg.Reset()
		if err != nil {
			return fmt.Errorf("resetting Avahi entry group is empty: %w", err)
		}
	}

	if status == "die" || status == "kill" || status == "pause" {
		return nil
	}

	container, err := docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspecting container: %w", err)
	}

	ips := extractIPNumbers(ctx, container)
	if len(ips) == 0 {
		return nil
	}

	hostname := extractHostname(ctx, container)
	services := extractServices(ctx, container)

	if hostname != "" {
		hostname = rewriteHostname(hostname)
		addToDNS(eg, hostname, ips, services, container.Name[1:], true)
	}

	containerHostname := rewriteHostname(container.Name[1:] + ".local")
	addToDNS(eg, containerHostname, ips, services, container.Name[1:], hostname == "")

	return nil
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
