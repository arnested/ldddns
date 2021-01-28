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

	_, err = daemon.SdNotify(true, daemon.SdNotifyReady)
	if err != nil {
		panic(fmt.Errorf("notifying systemd we're ready: %w", err))
	}

	err = conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"),
		dbus.WithMatchInterface("org.freedesktop.NetworkManager"),
		dbus.WithMatchSender("org.freedesktop.NetworkManager"),
	)
	if err != nil {
		panic(fmt.Errorf("add dbus NetworkManager signal matcher: %w", err))
	}

	// Do the magic work.
	go networkState(ctx, docker, egs, conn)
	handleExistingContainers(ctx, docker, egs)
	listen(ctx, docker, egs, started)
}

func handleExistingContainers(ctx context.Context, docker *client.Client, egs *EntryGroups) {
	containers, err := docker.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		logf(PriErr, "getting container list: %v", err)
	}

	for _, container := range containers {
		err = handleContainer(ctx, docker, container.ID, egs, "start")
		if err != nil {
			logf(PriErr, "handling container: %v", err)

			continue
		}
	}
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
			err := handleContainer(ctx, docker, msg.ID, egs, msg.Status)
			if err != nil {
				logf(PriErr, "handling container: %v", err)
			}
		case <-sig:
			logf(PriNotice, "Shutting down")
			os.Exit(int(syscall.SIGTERM))
		}
	}
}
