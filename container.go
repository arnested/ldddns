package main

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	internalContainer "ldddns.arnested.dk/internal/container"
	"ldddns.arnested.dk/internal/hostname"
	"ldddns.arnested.dk/internal/log"
	"ldddns.arnested.dk/internal/mdns"
)

// publisher publishes the mDNS records of containers.
type publisher interface {
	Publish(owner string, set mdns.RecordSet)
	Unpublish(owner string)
}

func handleContainer(
	ctx context.Context,
	docker *client.Client,
	containerID string,
	pub publisher,
	status events.Action,
	config Config,
) error {
	if status == "destroy" || status == "die" || status == "kill" || status == "pause" {
		pub.Unpublish(containerID)

		return nil
	}

	result, err := docker.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting container: %w", err)
	}

	containerInfo := internalContainer.Container{InspectResponse: result.Container}

	if ignoreOneoff(containerInfo, config) {
		return nil
	}

	addresses := ipv4Addresses(containerInfo.IPAddresses())
	if len(addresses) == 0 {
		pub.Unpublish(containerID)

		return nil
	}

	hostnames, err := hostname.Hostnames(containerInfo, config.HostnameLookup)
	if err != nil {
		return fmt.Errorf("getting hostnames: %w", err)
	}

	pub.Publish(containerID, mdns.RecordSet{
		Hostnames: hostnames,
		Addresses: addresses,
		Instance:  containerInfo.Name(),
		Services:  services(containerInfo.Services()),
	})

	return nil
}

func ipv4Addresses(ipNumbers []string) []netip.Addr {
	var addresses []netip.Addr

	for _, ipNumber := range ipNumbers {
		addr, err := netip.ParseAddr(ipNumber)
		if err != nil || !addr.Is4() {
			continue
		}

		addresses = append(addresses, addr)
	}

	return addresses
}

// services returns the services sorted by type, so that the same
// container gives the same record set.
func services(ports map[string]uint16) []mdns.Service {
	result := make([]mdns.Service, 0, len(ports))

	for _, serviceType := range slices.Sorted(maps.Keys(ports)) {
		result = append(result, mdns.Service{Type: serviceType, Port: ports[serviceType]})
	}

	return result
}

func ignoreOneoff(containerInfo internalContainer.Container, config Config) bool {
	if !config.IgnoreDockerComposeOneoff {
		return false
	}

	oneoff, ok := containerInfo.Config.Labels["com.docker.compose.oneoff"]
	if !ok {
		return false
	}

	if oneoff != "True" {
		return false
	}

	log.Logf(log.PriNotice, "Ignoring oneoff container: %s", containerInfo.ID)

	return true
}

func handleExistingContainers(ctx context.Context, config Config, docker *client.Client, pub publisher) {
	result, err := docker.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		log.Logf(log.PriErr, "getting container list: %v", err)
	}

	for _, container := range result.Items {
		if ctx.Err() != nil {
			return
		}

		err = handleContainer(ctx, docker, container.ID, pub, "start", config)
		if err != nil {
			log.Logf(log.PriErr, "handling container: %v", err)

			continue
		}
	}
}

func listen(ctx context.Context, config Config, docker *client.Client, pub publisher, started time.Time) {
	filter := make(client.Filters)
	filter.Add("type", "container")
	filter.Add("event", "destroy")
	filter.Add("event", "die")
	filter.Add("event", "kill")
	filter.Add("event", "pause")
	filter.Add("event", "start")
	filter.Add("event", "unpause")

	result := docker.Events(ctx, client.EventsListOptions{
		Filters: filter,
		Since:   strconv.FormatInt(started.Unix(), 10),
		Until:   "",
	})

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-result.Err:
			// A cancelled context closes the event stream with an error.
			if ctx.Err() != nil {
				return
			}

			panic(fmt.Errorf("go error reading docker events: %w", err))
		case msg := <-result.Messages:
			err := handleContainer(ctx, docker, msg.Actor.ID, pub, msg.Action, config)
			if err != nil {
				log.Logf(log.PriErr, "handling container: %v", err)
			}
		}
	}
}
