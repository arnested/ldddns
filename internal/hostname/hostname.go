package hostname

import (
	"regexp"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
	"ldddns.arnested.dk/internal/container"
	"ldddns.arnested.dk/internal/log"
)

const tld = ".local"

// Hostnames returns a slice of the hostnames we should use for the
// container.
func Hostnames(containerInfo container.Container, hostnameLookup []string) ([]string, error) {
	var hostnames []string

	for _, lookup := range hostnameLookup {
		switch {
		case lookup == "containerName":
			hostnames = append(hostnames, containerInfo.Name()+tld)

		case lookup[0:4] == "env:":
			hostnames = append(hostnames, containerInfo.HostnamesFromEnv(lookup[4:])...)

		case lookup[0:6] == "label:":
			hostnames = append(hostnames, containerInfo.HostnamesFromLabel(lookup[6:])...)
		}
	}

	// Each host name gives a flat name with one label below .local and
	// a name that keeps the dots. The flat name comes first, because the
	// services point to the first name, and the default configuration
	// of nss-mdns resolves only flat names.
	rewritten := make([]string, 0, 2*len(hostnames)) //nolint:mnd // Two names per host name.

	for _, hostname := range hostnames {
		rewritten = append(rewritten, RewriteHostname(hostname), RewriteHostnameLabels(hostname))
	}

	return removeDuplicates(rewritten), nil
}

// The regular expressions that sanitize a label.
var (
	invalidRunes = regexp.MustCompile(`[^\pL\d-]`)
	dashes       = regexp.MustCompile(`--+`)
	outerDashes  = regexp.MustCompile(`(^-+|-+$)`)
)

// RewriteHostname will make `hostname` suitable for dns-sd. The result
// has one label below .local, so `my.example.com` becomes
// `my-example.local`.
func RewriteHostname(hostname string) string {
	return rewrite(hostname, false)
}

// RewriteHostnameLabels makes `hostname` suitable for mDNS like
// RewriteHostname, but it keeps the dots between the labels, so
// `my.example.com` becomes `my.example.local`.
func RewriteHostnameLabels(hostname string) string {
	return rewrite(hostname, true)
}

func rewrite(hostname string, keepDots bool) string {
	profile := idna.New(
		idna.BidiRule(),
		idna.MapForLookup(),
		idna.RemoveLeadingDots(true),
		idna.StrictDomainName(true),
		idna.Transitional(true),
		idna.ValidateLabels(true),
	)

	// We ignore errors because we really just care about
	// converting legal punycode names into Unicode. The rest of
	// the function deals with turning the string into a valid
	// hostname.
	//nolint:errcheck
	unicodeHostname, _ := profile.ToUnicode(hostname)

	suffix, _ := publicsuffix.PublicSuffix(unicodeHostname)

	suffixRegExp := regexp.MustCompile(`\.` + regexp.QuoteMeta(suffix) + `$`)
	basename := suffixRegExp.ReplaceAllString(unicodeHostname, "")

	labels := []string{basename}
	if keepDots {
		labels = strings.Split(basename, ".")
	}

	sanitized := make([]string, 0, len(labels))

	for _, label := range labels {
		label = sanitizeLabel(label)
		if label != "" || !keepDots {
			sanitized = append(sanitized, label)
		}
	}

	sanitizedHostname := strings.Join(sanitized, ".") + tld

	sanitizedHostname, err := profile.ToASCII(sanitizedHostname)
	if err != nil {
		log.Logf(log.PriErr, "Could not rewrite hostname %q into proper IDNA", hostname)
	}

	if hostname != sanitizedHostname {
		log.Logf(log.PriInfo, "Rewrote hostname from %q to %q", hostname, sanitizedHostname)
	}

	return sanitizedHostname
}

// sanitizeLabel replaces the characters that are not letters, digits
// or dashes with dashes, and removes repeated, leading and trailing
// dashes.
func sanitizeLabel(label string) string {
	label = invalidRunes.ReplaceAllString(label, "-")
	label = dashes.ReplaceAllString(label, "-")

	return outerDashes.ReplaceAllString(label, "")
}

// removeDuplicates and keep the order.
func removeDuplicates(a []string) []string {
	result := []string{}
	seen := make(map[string]string, len(a))

	for _, val := range a {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = val
		}
	}

	return result
}
