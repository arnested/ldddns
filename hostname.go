package main

import (
	"context"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// rewriteHostname will make `hostname` suitable for dns-sd.
func rewriteHostname(_ context.Context, hostname string) string {
	suffix, _ := publicsuffix.PublicSuffix(hostname)
	basename := hostname[:len(hostname)-len(suffix)-1]
	sanitizedHostname := strings.ReplaceAll(basename, ".", "-") + ".local"

	if hostname != sanitizedHostname {
		logf(PriInfo, "Rewrote hostname from %q to %q\n", hostname, sanitizedHostname)
	}

	return sanitizedHostname
}
