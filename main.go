package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/carlmjohnson/versioninfo"
	"github.com/coreos/go-systemd/daemon"
	"github.com/docker/docker/client"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
	"github.com/kelseyhightower/envconfig"
	"ldddns.arnested.dk/internal/log"
)

var (
	//go:embed LICENSE
	//nolint:gochecknoglobals
	license string
	// Version string to be set at compile time via command line (-ldflags "-X main.version=1.2.3").
	version string
)

// Config is the configuration used to create hostnams for containers.
type Config struct {
	HostnameLookup            []string `split_words:"true" default:"env:VIRTUAL_HOST,containerName"`
	IgnoreDockerComposeOneoff bool     `split_words:"true" default:"true"`
}

func main() {
	version := getVersion()

	if len(os.Args) <= 1 || os.Args[1] != "start" {
		fmt.Fprintf(os.Stderr, "ldddns %s - https://ldddns.arnested.dk\n\n", version)
		fmt.Fprintln(os.Stderr, license)

		return
	}

	log.Logf(log.PriNotice, "Starting ldddns %s...", version)
	defer log.Logf(log.PriNotice, "Stopped ldddns %s.", version)

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

	err = sdNotify(daemon.SdNotifyReady, version, config)
	if err != nil {
		panic(fmt.Errorf("notifying systemd we're ready: %w", err))
	}

	// Do the magic work.
	handleExistingContainers(ctx, config, docker, egs)
	listen(ctx, config, docker, egs, started)

	err = sdNotify(daemon.SdNotifyStopping, version, config)
	if err != nil {
		log.Logf(log.PriErr, "notifying systemd we're shutting down: %v", err)
	}
}

func sdNotify(state string, version string, config Config) error {
	cfg, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("could not marshal config as JSON: %w", err)
	}

	_, err = daemon.SdNotify(true, fmt.Sprintf(
		"%s\nSTATUS=version %s; %s",
		state,
		version,
		cfg,
	))
	if err != nil {
		return fmt.Errorf("failed to notify systemd: %w", err)
	}

	return nil
}

func getVersion() string {
	buildinfo, _ := debug.ReadBuildInfo()

	if version == "" {
		version = versioninfo.Revision
	}

	if buildinfo.Main.Version != "(devel)" {
		version = buildinfo.Main.Version
	}

	if versioninfo.DirtyBuild {
		version = version + "-dirty"
	}

	return version
}
