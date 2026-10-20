package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/carlmjohnson/versioninfo"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/google/gops/agent"
	"github.com/kelseyhightower/envconfig"
	"github.com/moby/moby/client"
	"ldddns.arnested.dk/internal/log"
	"ldddns.arnested.dk/internal/mdns"
)

// shutdownTimeout is how long ldddns waits for a clean shutdown after
// SIGTERM. It is shorter than the stop timeout of systemd.
const shutdownTimeout = 10 * time.Second

var (
	//go:embed LICENSE.md
	license string
	// Version string to be set at compile time via command line (-ldflags "-X main.version=1.2.3").
	version string
)

// Config is the configuration used to create hostnams for containers.
//
//nolint:lll
type Config struct {
	Gops                      bool     `default:"false"                          json:"Gops"                      split_words:"true"`
	HostnameLookup            []string `default:"env:VIRTUAL_HOST,containerName" json:"HostnameLookup"            split_words:"true"`
	IgnoreDockerComposeOneoff bool     `default:"true"                           json:"IgnoreDockerComposeOneoff" split_words:"true"`
}

func main() {
	version := getVersion()

	if len(os.Args) <= 1 || os.Args[1] != "start" {
		fmt.Fprintf(os.Stderr, "ldddns %s - https://ldddns.arnested.dk\n\n%s", version, license)

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

	gops(config.Gops)

	docker, err := client.New(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("cannot create docker client: %w", err))
	}
	defer docker.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	go exitAfterShutdownTimeout(ctx, shutdownTimeout, os.Exit)

	responder, err := mdns.New(ctx)
	if err != nil {
		panic(fmt.Errorf("cannot start mDNS responder: %w", err))
	}

	defer func() {
		err := responder.Close()
		if err != nil {
			log.Logf(log.PriErr, "closing mDNS responder: %v", err)
		}
	}()

	started := time.Now()

	err = sdNotify(daemon.SdNotifyReady, version, config)
	if err != nil {
		panic(fmt.Errorf("notifying systemd we're ready: %w", err))
	}

	// Do the magic work.
	handleExistingContainers(ctx, config, docker, responder)
	listen(ctx, config, docker, responder, started)

	err = sdNotify(daemon.SdNotifyStopping, version, config)
	if err != nil {
		log.Logf(log.PriErr, "notifying systemd we're shutting down: %v", err)
	}
}

// exitAfterShutdownTimeout exits the process if a clean shutdown takes too
// long. It is a safety net, because all blocking calls take a context.
func exitAfterShutdownTimeout(ctx context.Context, timeout time.Duration, exit func(int)) {
	<-ctx.Done()
	time.Sleep(timeout)

	log.Logf(log.PriErr, "Shutdown did not finish within %s, exiting.", timeout)
	exit(1)
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
	if version == "" {
		version = versioninfo.Revision

		if versioninfo.DirtyBuild {
			version += "-dirty"
		}
	}

	buildinfo, ok := debug.ReadBuildInfo()

	if ok && (buildinfo != nil) && (buildinfo.Main.Version != "(devel)") {
		version = buildinfo.Main.Version
	}

	return version
}

// gops starts the gops agent if the config option is set.
func gops(start bool) {
	if !start {
		return
	}

	err := agent.Listen(agent.Options{
		ShutdownCleanup: true, // automatically closes on os.Interrupt
	})
	if err != nil {
		panic(fmt.Errorf("could not start gops agent: %w", err))
	}
}
