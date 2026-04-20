package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	internalContainer "ldddns.arnested.dk/internal/container"
	"ldddns.arnested.dk/internal/hostname"
	"ldddns.arnested.dk/internal/log"
)

//nolint:cyclop
func handleContainer(
	ctx context.Context,
	docker *client.Client,
	containerID string,
	egs *entryGroups,
	status events.Action,
	config Config,
) error {
	entryGroup, commit, err := egs.get(containerID)
	defer commit()

	if err != nil {
		return fmt.Errorf("cannot get entry group for container: %w", err)
	}

	empty, err := entryGroup.IsEmpty()
	if err != nil {
		return fmt.Errorf("checking whether Avahi entry group is empty: %w", err)
	}

	if !empty {
		err := entryGroup.Reset()
		if err != nil {
			return fmt.Errorf("resetting Avahi entry group is empty: %w", err)
		}
	}

	if status == "die" || status == "kill" || status == "pause" {
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

	ipNumbers := containerInfo.IPAddresses()
	if len(ipNumbers) == 0 {
		return nil
	}

	hostnames, err := hostname.Hostnames(containerInfo, config.HostnameLookup)
	if err != nil {
		return fmt.Errorf("getting hostnames: %w", err)
	}

	for _, hostname := range hostnames {
		addAddress(entryGroup, hostname, ipNumbers)
	}

	if services := containerInfo.Services(); len(hostnames) > 0 {
		addServices(entryGroup, hostnames[0], ipNumbers, services, containerInfo.Name())
	}

	return nil
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

func handleExistingContainers(ctx context.Context, config Config, docker *client.Client, egs *entryGroups) {
	result, err := docker.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		log.Logf(log.PriErr, "getting container list: %v", err)
	}

	for _, container := range result.Items {
		err = handleContainer(ctx, docker, container.ID, egs, "start", config)
		if err != nil {
			log.Logf(log.PriErr, "handling container: %v", err)

			continue
		}
	}
}

func listen(ctx context.Context, config Config, docker *client.Client, egs *entryGroups, started time.Time) {
	filter := make(client.Filters)
	filter.Add("type", "container")
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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)

	for {
		select {
		case err := <-result.Err:
			panic(fmt.Errorf("go error reading docker events: %w", err))
		case msg := <-result.Messages:
			err := handleContainer(ctx, docker, msg.Actor.ID, egs, events.Action(msg.Action), config)
			if err != nil {
				log.Logf(log.PriErr, "handling container: %v", err)
			}
		case <-sig:
			return
		}
	}
}
