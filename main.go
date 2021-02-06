package main

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
	"github.com/kelseyhightower/envconfig"
	"ldddns.arnested.dk/internal/log"
)

// Version string to be set at compile time via command line (-ldflags "-X main.version=1.2.3").
var (
	version string
)

type Config struct {
	HostnameLookup []string `split_words:"true" default:"env:VIRTUAL_HOST,containerName"`
}

func main() {
	log.Logf(log.PriNotice, "Starting ldddns v%s...", version)

	// Setup stuff.
	var config Config

	err := envconfig.Process("ldddns", &config)
	if err != nil {
		panic(fmt.Errorf("could not read environment config: %w", err))
	}

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

	// Do the magic work.
	handleExistingContainers(ctx, config, docker, egs)
	listen(ctx, config, docker, egs, started)
}
