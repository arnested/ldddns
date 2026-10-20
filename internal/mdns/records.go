package mdns

import (
	"net/netip"
	"slices"
	"strings"
	"unicode/utf8"

	"codeberg.org/miekg/dns"
)

const (
	// hostTTL is the TTL of records that hold a host name (RFC 6762
	// section 10).
	hostTTL = 120
	// otherTTL is the TTL of all other records.
	otherTTL = 4500
	// cacheFlush is the top bit of the class of a unique record in a
	// response. In a question the same bit asks for a unicast response.
	cacheFlush = 0x8000
	// maxLabel is the maximum length of a DNS label.
	maxLabel = 63

	domain       = "local."
	servicesName = "_services._dns-sd._udp." + domain
)

// Service is a DNS-SD service of a container.
type Service struct {
	// Type is the service type, for example "_http._tcp".
	Type string
	Port uint16
}

// RecordSet holds what one owner (a container) publishes.
type RecordSet struct {
	// Hostnames are the host names, for example "foo.local". The
	// services point to the first host name.
	Hostnames []string
	// Addresses are the IPv4 addresses of the host names.
	Addresses []netip.Addr
	// Instance is the DNS-SD instance name of the services.
	Instance string
	Services []Service
}

func (s RecordSet) equal(other RecordSet) bool {
	return slices.Equal(s.Hostnames, other.Hostnames) &&
		slices.Equal(s.Addresses, other.Addresses) &&
		s.Instance == other.Instance &&
		slices.Equal(s.Services, other.Services)
}

// record is one resource record that an owner publishes.
type record struct {
	res dns.RR
	// unique records get the cache-flush bit and are probed.
	unique bool
	// claim is the lowercase unique name the record depends on. The
	// record is active only while the owner holds that name.
	claim string
	// addr is the address of an A record.
	addr netip.Addr
}

// buildRecords returns the records of a record set.
func buildRecords(set RecordSet) []*record {
	addresses := ipv4Addresses(set.Addresses)
	if len(addresses) == 0 {
		return nil
	}

	records := hostRecords(set.Hostnames, addresses)

	if len(set.Hostnames) == 0 || set.Instance == "" {
		return records
	}

	for _, service := range set.Services {
		records = append(records, serviceRecords(service, set.Instance, fqdn(set.Hostnames[0]))...)
	}

	return records
}

// hostRecords returns an A record for each host name and address.
func hostRecords(hostnames []string, addresses []netip.Addr) []*record {
	records := make([]*record, 0, len(hostnames)*len(addresses))

	for _, hostname := range hostnames {
		name := fqdn(hostname)

		for _, addr := range addresses {
			records = append(records, &record{
				res: &dns.A{
					Hdr:  dns.Header{Name: name, TTL: hostTTL, Class: dns.ClassINET | cacheFlush},
					Addr: addr,
				},
				unique: true,
				claim:  strings.ToLower(name),
				addr:   addr,
			})
		}
	}

	return records
}

// serviceRecords returns the DNS-SD records of a service: PTR records
// for the service type and for the list of service types, and SRV and
// TXT records for the instance.
func serviceRecords(service Service, instanceName string, target string) []*record {
	serviceType := service.Type + "." + domain
	instance := instanceLabel(instanceName) + "." + serviceType
	claim := strings.ToLower(instance)

	return []*record{
		{
			res:    newPTR(serviceType, instance),
			unique: false,
			claim:  claim,
		},
		{
			res:    newPTR(servicesName, serviceType),
			unique: false,
			claim:  claim,
		},
		{
			res: &dns.SRV{
				Hdr:      dns.Header{Name: instance, TTL: hostTTL, Class: dns.ClassINET | cacheFlush},
				Priority: 0, Weight: 0, Port: service.Port, Target: target,
			},
			unique: true,
			claim:  claim,
		},
		{
			res: &dns.TXT{
				Hdr: dns.Header{Name: instance, TTL: otherTTL, Class: dns.ClassINET | cacheFlush},
				Txt: []string{""},
			},
			unique: true,
			claim:  claim,
		},
	}
}

func newPTR(name string, target string) *dns.PTR {
	return &dns.PTR{
		Hdr: dns.Header{Name: name, TTL: otherTTL, Class: dns.ClassINET},
		Ptr: target,
	}
}

func ipv4Addresses(addresses []netip.Addr) []netip.Addr {
	var result []netip.Addr

	for _, addr := range addresses {
		if addr.Is4() && !slices.Contains(result, addr) {
			result = append(result, addr)
		}
	}

	return result
}

func fqdn(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}

	return name + "."
}

// instanceLabel turns an instance name into one DNS label. The DNS
// library splits names at every dot, so a dot becomes a dash.
func instanceLabel(instance string) string {
	label := strings.ReplaceAll(instance, ".", "-")

	for len(label) > maxLabel {
		_, size := utf8.DecodeLastRuneInString(label)
		label = label[:len(label)-size]
	}

	return label
}
