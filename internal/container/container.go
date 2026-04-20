package container

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"honnef.co/go/netdb"
	"ldddns.arnested.dk/internal/log"
)

// Container holds information about a container.
type Container struct {
	container.InspectResponse
}

// Name is the containers name without the leading '/'.
func (c Container) Name() string {
	return c.InspectResponse.Name[1:]
}

// IPAddresses returns a slice of the IPv4 addresses of the container.
func (c Container) IPAddresses() []string {
	ips := []string{}

	for _, v := range c.NetworkSettings.Networks {
		if v.IPAddress.IsValid() {
			ips = append(ips, v.IPAddress.String())
		}
	}

	return ips
}

// Services from a container.
func (c Container) Services() map[string]uint16 {
	services := map[string]uint16{}

	for portProto := range c.NetworkSettings.Ports {
		port, protoName, found := strings.Cut(portProto.String(), "/")
		if !found {
			log.Logf(log.PriErr, "Port not found in: %q", portProto)

			continue
		}

		proto := netdb.GetProtoByName(protoName)

		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			log.Logf(log.PriErr, "Could not get port number from %q", portProto)

			continue
		}

		//nolint:mnd
		if portNumber > 65535 {
			log.Logf(log.PriErr, "Port number %d is too large", portNumber)

			continue
		}

		service := netdb.GetServByPort(int(portNumber), proto)

		if service == nil || proto == nil {
			continue
		}

		services[fmt.Sprintf("_%s._%s", service.Name, proto.Name)] = uint16(portNumber)
	}

	return services
}

// HostnamesFromEnv a container, return them as string slices.
func (c Container) HostnamesFromEnv(envName string) []string {
	prefix := envName + "="

	for _, s := range c.Config.Env {
		if strings.HasPrefix(s, prefix) {
			// Support multiple hostnames separated with comma and/or space.
			return strings.FieldsFunc(s[len(prefix):], func(r rune) bool { return r == ' ' || r == ',' })
		}
	}

	return []string{}
}

// HostnamesFromLabel a container, return them as string slices.
func (c Container) HostnamesFromLabel(label string) []string {
	if s, ok := c.Config.Labels[label]; ok {
		// Support multiple hostnames separated with comma and/or space.
		return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	}

	return []string{}
}
