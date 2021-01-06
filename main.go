package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// Version string to be set at compile time via command line (-ldflags "-X main.version=1.2.3").
var (
	version string
)

func main() {
	logf(PriNotice, "Starting ldddns %s...", version)
	// Setup stuff.
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("cannot create docker client: %w", err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dbus.SystemBus()
	if err != nil {
		panic(fmt.Errorf("cannot get dbus system bus: %w", err))
	}

	avahiServer, err := avahi.ServerNew(conn)
	if err != nil {
		panic(fmt.Errorf("avahi new failed: %w", err))
	}

	egs := NewEntryGroups(avahiServer)

	started := time.Now()

	_, _ = daemon.SdNotify(true, daemon.SdNotifyReady)

	// Do the magic work.
	containers, _ := docker.ContainerList(ctx, types.ContainerListOptions{})

	for _, container := range containers {
		containerJSON, _ := docker.ContainerInspect(ctx, container.ID)
		handleContainer(ctx, containerJSON, egs, "start")
	}

	listen(ctx, docker, egs, started)
}

func listen(ctx context.Context, docker *client.Client, egs *EntryGroups, started time.Time) {
	filter := filters.NewArgs()
	filter.Add("type", "container")
	filter.Add("event", "die")
	filter.Add("event", "kill")
	filter.Add("event", "pause")
	filter.Add("event", "start")
	filter.Add("event", "unpause")

	msgs, errs := docker.Events(ctx, types.EventsOptions{
		Filters: filter,
		Since:   strconv.FormatInt(started.Unix(), 10),
		Until:   "",
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)

	for {
		select {
		case err := <-errs:
			panic(fmt.Errorf("go error reading docker events: %w", err))
		case msg := <-msgs:
			handleMsg(ctx, docker, egs, msg.ID, msg.Status)
		case <-sig:
			logf(PriNotice, "Shutting down")
			os.Exit(int(syscall.SIGTERM))
		}
	}
}

// handleMsg handles an event message.
func handleMsg(ctx context.Context, docker *client.Client, egs *EntryGroups, id string, status string) {
	container, _ := docker.ContainerInspect(ctx, id)

	handleContainer(ctx, container, egs, status)
}
