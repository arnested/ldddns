//nolint:testpackage // The tests use a fake transport and the state of the responder.
package mdns

import (
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

const (
	testTimeout = 5 * time.Second

	ifLoopback = 1
	ifLAN      = 2
	ifBridge   = 3
)

//nolint:gochecknoglobals // Test fixtures.
var (
	containerAddr = netip.MustParseAddr("172.17.0.2")
	bridgeHost    = netip.MustParseAddrPort("172.17.0.1:5353")
	lanHost       = netip.MustParseAddr("192.168.1.10")
	lanPeer       = netip.MustParseAddr("192.168.1.20")

	testSet = RecordSet{
		Hostnames: []string{"foo.local"},
		Addresses: []netip.Addr{containerAddr},
		Instance:  "web",
		Services:  []Service{{Type: "_http._tcp", Port: 80}},
	}
)

type sentPacket struct {
	msg     *dns.Msg
	ifIndex int
	dst     netip.AddrPort
}

// fakeTransport is a transport that records the packets it sends and
// delivers the packets that a test injects.
type fakeTransport struct {
	in     chan packet
	done   chan struct{}
	once   sync.Once
	mutex  sync.Mutex
	sent   []sentPacket
	joined map[int]bool
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		in:     make(chan packet),
		done:   make(chan struct{}),
		once:   sync.Once{},
		mutex:  sync.Mutex{},
		sent:   nil,
		joined: make(map[int]bool),
	}
}

func (f *fakeTransport) readPacket() (packet, error) {
	select {
	case p := <-f.in:
		return p, nil
	case <-f.done:
		return packet{}, net.ErrClosed
	}
}

func (f *fakeTransport) writePacket(data []byte, ifIndex int, dst netip.AddrPort) error {
	msg := &dns.Msg{Data: slices.Clone(data)}

	err := msg.Unpack()
	if err != nil {
		return fmt.Errorf("unpacking sent packet: %w", err)
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.sent = append(f.sent, sentPacket{msg: msg, ifIndex: ifIndex, dst: dst})

	return nil
}

func (f *fakeTransport) join(ifIndex int) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.joined[ifIndex] = true

	return nil
}

func (f *fakeTransport) leave(ifIndex int) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	delete(f.joined, ifIndex)

	return nil
}

func (f *fakeTransport) close() error {
	f.once.Do(func() { close(f.done) })

	return nil
}

func (f *fakeTransport) packets() []sentPacket {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return slices.Clone(f.sent)
}

func (f *fakeTransport) reset() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.sent = nil
}

func (f *fakeTransport) isJoined(ifIndex int) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return f.joined[ifIndex]
}

func testInterfaces() ([]iface, error) {
	return []iface{
		{index: ifLoopback, name: "lo", multicast: false, prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/8")}},
		{index: ifLAN, name: "eth0", multicast: true, prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.1.10/24")}},
		{index: ifBridge, name: "docker0", multicast: true, prefixes: []netip.Prefix{netip.MustParsePrefix("172.17.0.1/16")}},
	}, nil
}

func testTiming() timing {
	return timing{
		probe:          time.Millisecond,
		announce:       time.Millisecond,
		tieBreak:       5 * time.Millisecond,
		rescan:         time.Hour,
		rateLimit:      0,
		probeRateLimit: 0,
		sharedDelay:    func() time.Duration { return 0 },
	}
}

func newTestResponder(t *testing.T, timing timing) (*Responder, *fakeTransport) {
	t.Helper()

	conn := newFakeTransport()
	responder := newResponder(conn, testInterfaces, timing)

	t.Cleanup(func() { _ = responder.Close() })

	return responder, conn
}

// waitFor polls until condition is true.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(time.Millisecond)
	}
}

func (r *Responder) stateOf(owner string, name string) (nameState, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ent, found := r.entries[owner]
	if !found {
		return 0, false
	}

	state, found := ent.states[name]

	return state, found
}

func (r *Responder) probingDone(owner string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ent := r.entries[owner]

	return ent != nil && !ent.probing
}

// publishLive publishes the test set and waits until probing and the
// announcements are done.
func publishLive(t *testing.T, responder *Responder, conn *fakeTransport) {
	t.Helper()

	responder.Publish("c1", testSet)
	waitFor(t, "probing", func() bool { return responder.probingDone("c1") })

	state, _ := responder.stateOf("c1", "foo.local.")
	if state != stateLive {
		t.Fatalf("Expected foo.local. to be live, got state %d", state)
	}

	conn.reset()
}

func packQuery(t *testing.T, id uint16, questions []dns.RR, known []dns.RR) []byte {
	t.Helper()

	msg := new(dns.Msg)
	msg.ID = id
	msg.Question = questions
	msg.Answer = known

	err := msg.Pack()
	if err != nil {
		t.Fatalf("packing query: %v", err)
	}

	return msg.Data
}

//nolint:ireturn // A question can be of any type.
func question(name string, qtype uint16) dns.RR {
	newRR := dns.TypeToRR[qtype]
	res := newRR()
	*res.Header() = dns.Header{Name: name, TTL: 0, Class: dns.ClassINET}

	return res
}

// query injects a query and returns the packets that the responder
// sends within a short time.
func query(t *testing.T, conn *fakeTransport, received packet) []sentPacket {
	t.Helper()

	conn.reset()

	conn.in <- received

	time.Sleep(20 * time.Millisecond)

	return conn.packets()
}

// checkProbe makes sure that a probe holds the proposed records and
// asks for a unicast response.
func checkProbe(t *testing.T, msg *dns.Msg) {
	t.Helper()

	if len(msg.Ns) == 0 {
		t.Error("Expected a probe to hold the proposed records")
	}

	for _, q := range msg.Question {
		if q.Header().Class&cacheFlush == 0 {
			t.Error("Expected a probe question to ask for a unicast response")
		}
	}
}

func TestPublishProbesAndAnnouncesOnBridgeOnly(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())

	responder.Publish("c1", testSet)
	waitFor(t, "probing", func() bool { return responder.probingDone("c1") })

	if !conn.isJoined(ifBridge) || conn.isJoined(ifLAN) || conn.isJoined(ifLoopback) {
		t.Error("Expected the responder to join the mDNS group on the bridge only")
	}

	probes, announcements := 0, 0

	for _, sent := range conn.packets() {
		if sent.ifIndex != ifBridge || sent.dst != groupDst() {
			t.Errorf("Expected multicast on the bridge only, got interface %d and %s", sent.ifIndex, sent.dst)
		}

		if sent.msg.Response {
			announcements++

			continue
		}

		probes++

		checkProbe(t, sent.msg)
	}

	if probes != probeCount {
		t.Errorf("Expected %d probes, got %d", probeCount, probes)
	}

	if announcements != 2 {
		t.Errorf("Expected 2 announcements, got %d", announcements)
	}
}

func TestQueryAnswersOnBridge(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	data := packQuery(t, 0, []dns.RR{question("FOO.local.", dns.TypeA)}, nil)

	sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost})
	if len(sent) != 1 {
		t.Fatalf("Expected 1 response, got %d", len(sent))
	}

	msg := sent[0].msg
	if !msg.Response || !msg.Authoritative || sent[0].dst != groupDst() {
		t.Error("Expected an authoritative multicast response")
	}

	a, ok := msg.Answer[0].(*dns.A)
	if !ok || a.Addr != containerAddr || a.Hdr.Class != dns.ClassINET|cacheFlush || a.Hdr.TTL != hostTTL {
		t.Errorf("Expected a unique A record for %s, got %v", containerAddr, msg.Answer[0])
	}

	sent = query(t, conn, packet{data: data, ifIndex: ifLAN, src: netip.AddrPortFrom(lanPeer, port)})
	if len(sent) != 0 {
		t.Errorf("Expected no response on the LAN, got %d", len(sent))
	}
}

func TestKnownAnswerSuppression(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	known := &dns.A{
		Hdr:  dns.Header{Name: "foo.local.", TTL: hostTTL, Class: dns.ClassINET},
		Addr: containerAddr,
	}

	data := packQuery(t, 0, []dns.RR{question("foo.local.", dns.TypeA)}, []dns.RR{known})
	if sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost}); len(sent) != 0 {
		t.Errorf("Expected a known answer to get no response, got %d", len(sent))
	}

	known.Hdr.TTL = hostTTL/2 - 1

	data = packQuery(t, 0, []dns.RR{question("foo.local.", dns.TypeA)}, []dns.RR{known})
	if sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost}); len(sent) != 1 {
		t.Errorf("Expected a known answer with a low TTL to get a response, got %d", len(sent))
	}
}

func TestLegacyUnicast(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	data := packQuery(t, 4711, []dns.RR{question("foo.local.", dns.TypeA)}, nil)

	tests := []struct {
		name     string
		ifIndex  int
		src      netip.AddrPort
		expected bool
	}{
		{name: "from the bridge", ifIndex: ifBridge, src: netip.MustParseAddrPort("172.17.0.1:40000"), expected: true},
		{name: "from this host on the LAN", ifIndex: ifLAN, src: netip.AddrPortFrom(lanHost, 40000), expected: true},
		{name: "from another host on the LAN", ifIndex: ifLAN, src: netip.AddrPortFrom(lanPeer, 40000), expected: false},
	}

	for _, testCase := range tests {
		sent := query(t, conn, packet{data: data, ifIndex: testCase.ifIndex, src: testCase.src})

		if !testCase.expected {
			if len(sent) != 0 {
				t.Errorf("%s: expected no response, got %d", testCase.name, len(sent))
			}

			continue
		}

		if len(sent) != 1 {
			t.Fatalf("%s: expected 1 response, got %d", testCase.name, len(sent))
		}

		msg := sent[0].msg

		if sent[0].dst != testCase.src || msg.ID != 4711 || len(msg.Question) != 1 {
			t.Errorf("%s: expected a unicast response with the query ID and question", testCase.name)
		}

		header := msg.Answer[0].Header()
		if header.TTL != legacyTTL || header.Class != dns.ClassINET {
			t.Errorf("%s: expected TTL %d and no cache-flush bit, got %v", testCase.name, legacyTTL, msg.Answer[0])
		}
	}
}

func TestServiceQueryAddsAdditionalRecords(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	data := packQuery(t, 0, []dns.RR{question("_http._tcp.local.", dns.TypePTR)}, nil)

	sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost})
	if len(sent) != 1 {
		t.Fatalf("Expected 1 response, got %d", len(sent))
	}

	msg := sent[0].msg

	ptr, ok := msg.Answer[0].(*dns.PTR)
	if !ok || ptr.Ptr != "web._http._tcp.local." || ptr.Hdr.Class != dns.ClassINET {
		t.Errorf("Expected a shared PTR record, got %v", msg.Answer[0])
	}

	types := make([]uint16, 0, len(msg.Extra))
	for _, res := range msg.Extra {
		types = append(types, rrType(res))
	}

	slices.Sort(types)

	if !slices.Equal(types, []uint16{dns.TypeA, dns.TypeTXT, dns.TypeSRV}) {
		t.Errorf("Expected additional A, SRV and TXT records, got %v", msg.Extra)
	}

	data = packQuery(t, 0, []dns.RR{question(servicesName, dns.TypePTR)}, nil)

	sent = query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost})
	if len(sent) != 1 {
		t.Fatalf("Expected 1 response to the service type query, got %d", len(sent))
	}

	if ptr, ok := sent[0].msg.Answer[0].(*dns.PTR); !ok || ptr.Ptr != "_http._tcp.local." {
		t.Errorf("Expected the service type, got %v", sent[0].msg.Answer[0])
	}
}

func TestNSECForMissingType(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	data := packQuery(t, 0, []dns.RR{question("foo.local.", dns.TypeAAAA)}, nil)

	sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost})
	if len(sent) != 1 {
		t.Fatalf("Expected 1 response, got %d", len(sent))
	}

	nsecRR, ok := sent[0].msg.Answer[0].(*dns.NSEC)
	if !ok || !slices.Equal(nsecRR.TypeBitMap, []uint16{dns.TypeA}) {
		t.Errorf("Expected an NSEC record with type A, got %v", sent[0].msg.Answer[0])
	}

	data = packQuery(t, 0, []dns.RR{question("bar.local.", dns.TypeAAAA)}, nil)
	if sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost}); len(sent) != 0 {
		t.Errorf("Expected no response for an unknown name, got %d", len(sent))
	}
}

func TestRateLimit(t *testing.T) {
	t.Parallel()

	timing := testTiming()
	timing.rateLimit = time.Hour

	responder, conn := newTestResponder(t, timing)
	publishLive(t, responder, conn)

	data := packQuery(t, 0, []dns.RR{question("foo.local.", dns.TypeA)}, nil)
	if sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost}); len(sent) != 0 {
		t.Errorf("Expected the announced record to be rate limited, got %d responses", len(sent))
	}
}

func TestUnpublishSendsGoodbye(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	responder.Unpublish("c1")

	sent := conn.packets()
	if len(sent) == 0 {
		t.Fatal("Expected goodbye packets")
	}

	for _, packet := range sent {
		for _, res := range packet.msg.Answer {
			if res.Header().TTL != 0 {
				t.Errorf("Expected TTL 0 in a goodbye, got %v", res)
			}
		}
	}

	if conn.isJoined(ifBridge) {
		t.Error("Expected the responder to leave the mDNS group")
	}

	data := packQuery(t, 0, []dns.RR{question("foo.local.", dns.TypeA)}, nil)
	if sent := query(t, conn, packet{data: data, ifIndex: ifBridge, src: bridgeHost}); len(sent) != 0 {
		t.Errorf("Expected no response after unpublish, got %d", len(sent))
	}
}

func TestPublishSameSetDoesNothing(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	responder.Publish("c1", testSet)

	if sent := conn.packets(); len(sent) != 0 {
		t.Errorf("Expected no packets when the set did not change, got %d", len(sent))
	}

	if state, _ := responder.stateOf("c1", "foo.local."); state != stateLive {
		t.Errorf("Expected the name to stay live, got state %d", state)
	}
}

func TestConflictBetweenOwners(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	other := testSet
	other.Addresses = []netip.Addr{netip.MustParseAddr("172.17.0.3")}
	other.Instance = "other"

	responder.Publish("c2", other)

	if state, _ := responder.stateOf("c2", "foo.local."); state != stateConflict {
		t.Errorf("Expected a conflict for the second owner, got state %d", state)
	}
}

func TestConflictWhileProbing(t *testing.T) {
	t.Parallel()

	timing := testTiming()
	timing.probe = 50 * time.Millisecond

	responder, conn := newTestResponder(t, timing)

	responder.Publish("c1", testSet)

	response := newResponse()
	response.Answer = []dns.RR{&dns.A{
		Hdr:  dns.Header{Name: "foo.local.", TTL: hostTTL, Class: dns.ClassINET | cacheFlush},
		Addr: netip.MustParseAddr("172.17.0.99"),
	}}

	err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}

	conn.in <- packet{data: response.Data, ifIndex: ifBridge, src: netip.MustParseAddrPort("172.17.0.99:5353")}

	waitFor(t, "probing", func() bool { return responder.probingDone("c1") })

	if state, _ := responder.stateOf("c1", "foo.local."); state != stateConflict {
		t.Errorf("Expected a conflict, got state %d", state)
	}

	if state, _ := responder.stateOf("c1", "web._http._tcp.local."); state != stateLive {
		t.Errorf("Expected the service name to go live, got state %d", state)
	}
}

func TestOwnPacketsAreNoConflict(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	// An announcement of the responder, looped back.
	response := newResponse()

	responder.mutex.Lock()
	ent := responder.entries["c1"]
	responder.mutex.Unlock()

	if ent == nil {
		t.Fatal("Expected an entry for c1")
	}

	for _, rec := range ent.records {
		response.Answer = append(response.Answer, rec.res)
	}

	err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}

	query(t, conn, packet{data: response.Data, ifIndex: ifBridge, src: bridgeHost})

	if state, _ := responder.stateOf("c1", "foo.local."); state != stateLive {
		t.Errorf("Expected our own records to cause no conflict, got state %d", state)
	}
}

func TestLiveConflictProbesAgain(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	response := newResponse()
	response.Answer = []dns.RR{&dns.A{
		Hdr:  dns.Header{Name: "foo.local.", TTL: hostTTL, Class: dns.ClassINET | cacheFlush},
		Addr: netip.MustParseAddr("172.17.0.99"),
	}}

	err := response.Pack()
	if err != nil {
		t.Fatal(err)
	}

	conn.in <- packet{data: response.Data, ifIndex: ifBridge, src: netip.MustParseAddrPort("172.17.0.99:5353")}

	waitFor(t, "probes", func() bool {
		for _, sent := range conn.packets() {
			if !sent.msg.Response {
				return true
			}
		}

		return false
	})
}

func TestTieBreak(t *testing.T) {
	t.Parallel()

	responder, _ := newTestResponder(t, testTiming())

	responder.mutex.Lock()

	// Publish without the probe goroutine, so the test controls the
	// state.
	ent := &entry{
		owner:        "c1",
		set:          testSet,
		records:      buildRecords(testSet),
		states:       map[string]nameState{"foo.local.": stateProbing},
		probing:      true,
		lostTieBreak: false,
	}
	responder.entries["c1"] = ent

	probe := func(addr string) *dns.Msg {
		msg := new(dns.Msg)
		msg.Question = []dns.RR{question("foo.local.", dns.TypeANY)}
		msg.Ns = []dns.RR{&dns.A{
			Hdr:  dns.Header{Name: "foo.local.", TTL: hostTTL, Class: dns.ClassINET},
			Addr: netip.MustParseAddr(addr),
		}}

		return msg
	}

	other := probe("172.17.0.200")
	other.Ns[0].Header().Name = "other.local."

	responder.tieBreakLocked(other, ifBridge)

	if ent.lostTieBreak {
		t.Error("Expected a probe for another name to cause no tie-break")
	}

	responder.tieBreakLocked(probe(containerAddr.String()), ifBridge)

	if ent.lostTieBreak {
		t.Error("Expected our own probe to cause no tie-break")
	}

	responder.tieBreakLocked(probe("172.17.0.1"), ifBridge)

	if ent.lostTieBreak {
		t.Error("Expected a lower address to lose the tie-break")
	}

	responder.tieBreakLocked(probe("172.17.0.200"), ifBridge)

	if !ent.lostTieBreak {
		t.Error("Expected a higher address to win the tie-break")
	}

	responder.mutex.Unlock()
}

func TestCloseSendsGoodbyes(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())
	publishLive(t, responder, conn)

	err := responder.Close()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(conn.packets()) == 0 {
		t.Error("Expected goodbye packets on close")
	}

	// Calls after Close do nothing.
	responder.Publish("c2", testSet)
	responder.Unpublish("c1")

	err = responder.Close()
	if err != nil {
		t.Errorf("Expected a second close to return no error, got %v", err)
	}
}

func TestPublishWithoutAddresses(t *testing.T) {
	t.Parallel()

	responder, conn := newTestResponder(t, testTiming())

	responder.Publish("c1", RecordSet{Hostnames: []string{"foo.local"}, Addresses: nil, Instance: "", Services: nil})

	if _, ok := responder.stateOf("c1", "foo.local."); ok {
		t.Error("Expected no entry for a set without addresses")
	}

	if conn.isJoined(ifBridge) {
		t.Error("Expected no group membership")
	}
}
