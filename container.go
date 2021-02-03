package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
	"ldddns.arnested.dk/internal/container"
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

	containerJSON, err := docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspecting container: %w", err)
	}

	c := container.Container{ContainerJSON: containerJSON}

	ips := c.IPAddresses()
	if len(ips) == 0 {
		return nil
	}

	hostnames := c.HostnamesFromEnv("VIRTUAL_HOST")
	services := c.Services()

	for i, hostname := range hostnames {
		hostname = rewriteHostname(hostname)
		addToDNS(eg, hostname, ips, services, c.Name(), i == 0)
	}

	containerHostname := rewriteHostname(c.Name() + ".local")
	addToDNS(eg, containerHostname, ips, services, c.Name(), len(hostnames) == 0)

	return nil
}
