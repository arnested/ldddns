package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
	"ldddns.arnested.dk/internal/log"
)

var (
	//go:embed LICENSE
	//nolint:gochecknoglobals
	license string
	// Version string to be set at compile time via command line (-ldflags "-X main.version=1.2.3").
	version = "DEV"
)

func main() {
	configUnit := flag.String("config", "", "the name of the systemd unit with config")
	start := flag.Bool("start", false, "start the service")
	flag.Parse()

	if !*start {
		fmt.Fprintf(os.Stderr, "ldddns v%s (https://ldddns.arnested.dk)\n\n%s\n", version, license)

		return
	}

	log.Logf(log.PriNotice, "Starting ldddns v%s...", version)
	defer log.Logf(log.PriNotice, "Stopped ldddns v%s.", version)

	config, err := config(*configUnit)
	if err != nil {
		panic(fmt.Errorf("cannot get config: %w", err))
	}

	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("cannot create docker client: %w", err))
	}
	defer docker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := dbus.SystemBus()
	if err != nil {
		panic(fmt.Errorf("cannot get dbus system bus: %w", err))
	}
	defer conn.Close()

	avahiServer, err := avahi.ServerNew(conn)
	if err != nil {
		panic(fmt.Errorf("avahi new failed: %w", err))
	}
	defer avahiServer.Close()

	egs := newEntryGroups(avahiServer)

	started := time.Now()

	_, err = daemon.SdNotify(true, daemon.SdNotifyReady)
	if err != nil {
		panic(fmt.Errorf("notifying systemd we're ready: %w", err))
	}

	// Do the magic work.
	handleExistingContainers(ctx, config, docker, egs)
	listen(ctx, config, docker, egs, started)

	_, err = daemon.SdNotify(true, daemon.SdNotifyStopping)
	if err != nil {
		log.Logf(log.PriErr, "notifying systemd we're shutting down: %v", err)
	}
}
