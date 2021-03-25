package main

import (
	"fmt"
	"os/exec"

	"gopkg.in/ini.v1"
)

// Config is the configuration used to create hostnams for containers.
type Config struct {
	HostnameLookup   []string
	BroadcastService bool
}

func config(configUnit string) (Config, error) {
	// Setup stuff.
	var config Config

	unit, err := exec.Command("systemctl", "cat", configUnit).Output()
	if err != nil {
		return config, fmt.Errorf("getting systemd unit: %w", err)
	}

	cfg, err := ini.Load(unit)
	if err != nil {
		return config, fmt.Errorf("parsing systemd unit: %w", err)
	}

	section := cfg.Section("X-ldddns")
	config.HostnameLookup = section.Key("HostnameLookup").Strings(",")
	config.BroadcastService = section.Key("BroadcastService").MustBool(true)

	return config, nil
}
