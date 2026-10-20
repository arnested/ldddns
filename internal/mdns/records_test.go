//nolint:testpackage // The tests use unexported functions.
package mdns

import (
	"maps"
	"net/netip"
	"strings"
	"testing"

	"codeberg.org/miekg/dns"
)

func TestBuildRecords(t *testing.T) {
	t.Parallel()

	set := RecordSet{
		Hostnames: []string{"foo.local", "bar.local"},
		Addresses: []netip.Addr{containerAddr, netip.MustParseAddr("fd00::2"), containerAddr},
		Instance:  "my.web app",
		Services:  []Service{{Type: "_http._tcp", Port: 80}},
	}

	counts := make(map[uint16]int)

	for _, rec := range buildRecords(set) {
		counts[rrType(rec.res)]++

		if srv, ok := rec.res.(*dns.SRV); ok {
			if srv.Hdr.Name != "my-web app._http._tcp.local." || srv.Target != "foo.local." || srv.Port != 80 {
				t.Errorf("Unexpected SRV record %v", srv)
			}
		}
	}

	// One A record per host name, because the IPv6 address and the
	// duplicate are left out.
	expected := map[uint16]int{dns.TypeA: 2, dns.TypePTR: 2, dns.TypeSRV: 1, dns.TypeTXT: 1}
	if !maps.Equal(counts, expected) {
		t.Errorf("Expected record counts %v, got %v", expected, counts)
	}
}

func TestBuildRecordsWithoutAddresses(t *testing.T) {
	t.Parallel()

	records := buildRecords(RecordSet{Hostnames: []string{"foo.local"}, Addresses: nil, Instance: "", Services: nil})
	if records != nil {
		t.Errorf("Expected no records without addresses, got %v", records)
	}
}

func TestRecordsPackAndUnpack(t *testing.T) {
	t.Parallel()

	set := testSet
	set.Hostnames = []string{"www.my-project.local", "www-my-project.local"}

	msg := newResponse()

	for _, rec := range buildRecords(set) {
		msg.Answer = append(msg.Answer, rec.res)
	}

	err := msg.Pack()
	if err != nil {
		t.Fatalf("packing: %v", err)
	}

	unpacked := &dns.Msg{Data: msg.Data}

	err = unpacked.Unpack()
	if err != nil {
		t.Fatalf("unpacking: %v", err)
	}

	for i, res := range unpacked.Answer {
		if key(res) != key(msg.Answer[i]) {
			t.Errorf("Expected %v after a round trip, got %v", msg.Answer[i], res)
		}
	}
}

func TestInstanceLabel(t *testing.T) {
	t.Parallel()

	if got := instanceLabel("a.b"); got != "a-b" {
		t.Errorf("Expected a-b, got %q", got)
	}

	long := strings.Repeat("æ", 40)
	if got := instanceLabel(long); len(got) > maxLabel || got != strings.Repeat("æ", 31) {
		t.Errorf("Expected a label of at most %d bytes on a rune boundary, got %q", maxLabel, got)
	}
}

func TestCompareRecordSets(t *testing.T) {
	t.Parallel()

	aRecord := func(addr string) dns.RR {
		return &dns.A{Hdr: dns.Header{Name: "foo.local.", Class: dns.ClassINET}, Addr: netip.MustParseAddr(addr)}
	}

	tests := []struct {
		name   string
		ours   []dns.RR
		theirs []dns.RR
		want   int
	}{
		{name: "equal", ours: []dns.RR{aRecord("10.0.0.1")}, theirs: []dns.RR{aRecord("10.0.0.1")}, want: 0},
		{name: "lower loses", ours: []dns.RR{aRecord("10.0.0.1")}, theirs: []dns.RR{aRecord("10.0.0.2")}, want: -1},
		{name: "higher wins", ours: []dns.RR{aRecord("10.0.0.3")}, theirs: []dns.RR{aRecord("10.0.0.2")}, want: 1},
		{
			name:   "longer wins",
			ours:   []dns.RR{aRecord("10.0.0.2"), aRecord("10.0.0.1")},
			theirs: []dns.RR{aRecord("10.0.0.1")},
			want:   1,
		},
	}

	for _, testCase := range tests {
		got := compareRecordSets(testCase.ours, testCase.theirs)
		if (got < 0) != (testCase.want < 0) || (got > 0) != (testCase.want > 0) {
			t.Errorf("%s: expected %d, got %d", testCase.name, testCase.want, got)
		}
	}
}

func TestBuildResponsesSplits(t *testing.T) {
	t.Parallel()

	answers := make([]dns.RR, 0, 200)

	for i := range 200 {
		answers = append(answers, &dns.A{
			Hdr:  dns.Header{Name: "host-" + strings.Repeat("x", i%40) + ".local.", TTL: hostTTL, Class: dns.ClassINET},
			Addr: netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i)}),
		})
	}

	msgs := buildResponses(answers, answers[:1])

	total := 0

	for _, msg := range msgs {
		if msg.Len() > maxResponse {
			t.Errorf("Expected responses of at most %d bytes, got %d", maxResponse, msg.Len())
		}

		total += len(msg.Answer)
	}

	if len(msgs) < 2 || total != len(answers) {
		t.Errorf("Expected all %d answers in more responses, got %d in %d", len(answers), total, len(msgs))
	}

	if buildResponses(nil, answers) != nil {
		t.Error("Expected no responses without answers")
	}
}
