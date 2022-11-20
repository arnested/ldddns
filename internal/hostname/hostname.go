package hostname

import (
	"regexp"

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

	for i, hostname := range hostnames {
		hostnames[i] = RewriteHostname(hostname)
	}

	return removeDuplicates(hostnames), nil
}

// RewriteHostname will make `hostname` suitable for dns-sd.
func RewriteHostname(hostname string) string {
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

	suffixRegExp = regexp.MustCompile(`[^\pL\d-]`)
	basename = suffixRegExp.ReplaceAllString(basename, "-")

	suffixRegExp = regexp.MustCompile(`--+`)
	basename = suffixRegExp.ReplaceAllString(basename, "-")

	suffixRegExp = regexp.MustCompile(`(^-+|-+$)`)
	basename = suffixRegExp.ReplaceAllString(basename, "")

	sanitizedHostname := basename + tld

	sanitizedHostname, err := profile.ToASCII(sanitizedHostname)
	if err != nil {
		log.Logf(log.PriErr, "Could not rewrite hostname %q into proper IDNA", hostname)
	}

	if hostname != sanitizedHostname {
		log.Logf(log.PriInfo, "Rewrote hostname from %q to %q", hostname, sanitizedHostname)
	}

	return sanitizedHostname
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
