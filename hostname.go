package main

import (
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"golang.org/x/net/publicsuffix"
	"ldddns.arnested.dk/internal/container"
)

type config struct {
	HostnameLookup []string `split_words:"true" default:"env:VIRTUAL_HOST,containerName"`
}

func hostnames(c container.Container) ([]string, error) {
	var config config

	err := envconfig.Process("ldddns", &config)
	if err != nil {
		return []string{}, fmt.Errorf("could not read env config: %w", err)
	}

	var hostnames []string

	for _, lookup := range config.HostnameLookup {
		switch {
		case lookup == "containerName":
			hostnames = append(hostnames, c.Name()+".local")

		case lookup[0:4] == "env:":
			hostnames = append(hostnames, c.HostnamesFromEnv(lookup[4:])...)
		}
	}

	for i, hostname := range hostnames {
		hostnames[i] = rewriteHostname(hostname)
	}

	return hostnames, nil
}

// rewriteHostname will make `hostname` suitable for dns-sd.
func rewriteHostname(hostname string) string {
	suffix, _ := publicsuffix.PublicSuffix(hostname)
	basename := hostname[:len(hostname)-len(suffix)-1]
	basename = strings.ReplaceAll(basename, ".", "-")
	basename = strings.ReplaceAll(basename, "_", "-")
	sanitizedHostname := basename + ".local"

	if hostname != sanitizedHostname {
		logf(PriInfo, "Rewrote hostname from %q to %q", hostname, sanitizedHostname)
	}

	return sanitizedHostname
}
