package main

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

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
