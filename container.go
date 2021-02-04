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

	hostnames, err := hostnames(c)
	if err != nil {
		return fmt.Errorf("getting hostnames: %w", err)
	}

	for _, hostname := range hostnames {
		addAddress(eg, hostname, ips)
	}

	if services := c.Services(); len(hostnames) > 0 {
		addServices(eg, hostnames[0], ips, services, c.Name())
	}

	return nil
}
