// Package mdns is a Multicast DNS responder (RFC 6762) with DNS-SD
// records (RFC 6763) for the containers of a Docker host.
//
// The responder answers a record only on the interface whose IPv4
// subnet holds the address of the container. Docker bridges are
// host-only networks, so the names do not reach the LAN.
package mdns

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"

	"codeberg.org/miekg/dns"
	"ldddns.arnested.dk/internal/log"
)

// nameState is the state of a unique name of an owner.
type nameState int

const (
	// The zero value is no state, so that a name that is missing from
	// the states of an entry is in none of the states below.
	_ nameState = iota
	// stateProbing means that the responder probes the name and does
	// not answer for it yet.
	stateProbing
	// stateLive means that the responder answers for the name.
	stateLive
	// stateConflict means that another host or owner has the name.
	stateConflict
)

// probeCount is the number of probes before a name goes live (RFC 6762
// section 8.1).
const probeCount = 3

// timing holds the intervals of the responder.
type timing struct {
	probe    time.Duration
	announce time.Duration
	tieBreak time.Duration
	rescan   time.Duration
	// rateLimit is the minimum time between two multicasts of a record
	// on an interface (RFC 6762 section 6.2).
	rateLimit time.Duration
	// probeRateLimit is the rate limit for answers to probes.
	probeRateLimit time.Duration
	// sharedDelay returns the delay of a response with shared records.
	sharedDelay func() time.Duration
}

//nolint:gochecknoglobals,mnd // The default intervals of RFC 6762.
var defaultTiming = timing{
	probe:          250 * time.Millisecond,
	announce:       time.Second,
	tieBreak:       time.Second,
	rescan:         30 * time.Second,
	rateLimit:      time.Second,
	probeRateLimit: 250 * time.Millisecond,
	sharedDelay: func() time.Duration {
		return 20*time.Millisecond + rand.N(100*time.Millisecond) //nolint:gosec // Jitter only.
	},
}

// entry is the published state of one owner.
type entry struct {
	owner   string
	set     RecordSet
	records []*record
	states  map[string]nameState
	// probing is true while a probe goroutine runs for the entry.
	probing bool
	// lostTieBreak is true when a simultaneous probe of another host
	// won the tie-break.
	lostTieBreak bool
}

func (ent *entry) active(rec *record) bool {
	return ent.states[rec.claim] == stateLive
}

func (ent *entry) namesIn(state nameState) []string {
	var names []string

	for name, current := range ent.states {
		if current == state {
			names = append(names, name)
		}
	}

	return names
}

// Responder is a Multicast DNS responder.
type Responder struct {
	conn           transport
	listInterfaces func() ([]iface, error)
	timing         timing

	ctx    context.Context //nolint:containedctx // Stops the goroutines of the responder.
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mutex     sync.Mutex
	entries   map[string]*entry
	ifaces    map[int]iface
	hostAddrs map[netip.Addr]bool
	joined    map[int]bool
	// joinFailed holds the interfaces where a join failed.
	joinFailed map[int]bool
	lastSent   map[sentKey]time.Time
	closed     bool
}

type sentKey struct {
	ifIndex int
	key     string
}

// New opens the mDNS socket and starts a responder.
func New(ctx context.Context) (*Responder, error) {
	conn, err := listen(ctx)
	if err != nil {
		return nil, err
	}

	return newResponder(conn, systemInterfaces, defaultTiming), nil
}

func newResponder(conn transport, listInterfaces func() ([]iface, error), timing timing) *Responder {
	ctx, cancel := context.WithCancel(context.Background())

	responder := &Responder{
		conn:           conn,
		listInterfaces: listInterfaces,
		timing:         timing,
		ctx:            ctx,
		cancel:         cancel,
		wg:             sync.WaitGroup{},
		mutex:          sync.Mutex{},
		entries:        make(map[string]*entry),
		ifaces:         make(map[int]iface),
		hostAddrs:      make(map[netip.Addr]bool),
		joined:         make(map[int]bool),
		joinFailed:     make(map[int]bool),
		lastSent:       make(map[sentKey]time.Time),
		closed:         false,
	}

	responder.mutex.Lock()
	responder.refreshInterfacesLocked()
	responder.mutex.Unlock()

	responder.wg.Go(responder.readLoop)
	responder.wg.Go(responder.rescanLoop)

	return responder
}

// Publish publishes the records of an owner. It replaces the records
// that the owner published before.
func (r *Responder) Publish(owner string, set RecordSet) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return
	}

	old, ok := r.entries[owner]
	if ok && old.set.equal(set) {
		return
	}

	if ok {
		r.removeLocked(old)
	}

	records := buildRecords(set)
	if len(records) == 0 {
		r.refreshInterfacesLocked()

		return
	}

	newEntry := &entry{
		owner:        owner,
		set:          set,
		records:      records,
		states:       make(map[string]nameState),
		probing:      false,
		lostTieBreak: false,
	}

	for _, rec := range records {
		if _, seen := newEntry.states[rec.claim]; seen {
			continue
		}

		newEntry.states[rec.claim] = stateProbing

		if other := r.ownerOfLocked(rec.claim); other != "" {
			log.Logf(log.PriWarning, "mDNS name %s of container %s is already used by container %s",
				rec.claim, owner, other)

			newEntry.states[rec.claim] = stateConflict
		}
	}

	r.entries[owner] = newEntry
	r.refreshInterfacesLocked()
	r.startProbeLocked(newEntry)
}

// Unpublish removes the records of an owner and sends goodbye packets
// for them.
func (r *Responder) Unpublish(owner string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return
	}

	old, ok := r.entries[owner]
	if !ok {
		return
	}

	r.removeLocked(old)
	r.refreshInterfacesLocked()
}

// Close sends goodbye packets for all records and stops the responder.
func (r *Responder) Close() error {
	r.mutex.Lock()

	if r.closed {
		r.mutex.Unlock()

		return nil
	}

	r.closed = true

	for _, old := range r.entries {
		r.removeLocked(old)
	}

	r.mutex.Unlock()

	r.cancel()
	err := r.conn.close()
	r.wg.Wait()

	return err
}

// removeLocked forgets an entry and sends goodbye packets (TTL 0) for
// its live records (RFC 6762 section 10.1).
func (r *Responder) removeLocked(old *entry) {
	delete(r.entries, old.owner)

	for _, ifi := range r.multicastIfacesLocked() {
		live := r.recordsOnLocked(old, ifi, old.active)
		goodbyes := make([]dns.RR, 0, len(live))

		for _, res := range live {
			goodbye := res.Clone()
			goodbye.Header().TTL = 0
			goodbyes = append(goodbyes, goodbye)
		}

		r.sendResponseLocked(ifi.index, groupDst(), goodbyes, nil)
	}

	log.Logf(log.PriDebug, "unpublished mDNS records of container %s", old.owner)
}

// ownerOfLocked returns the owner that holds a unique name.
func (r *Responder) ownerOfLocked(name string) string {
	for _, other := range r.entries {
		state, ok := other.states[name]
		if ok && state != stateConflict {
			return other.owner
		}
	}

	return ""
}

func (r *Responder) current(ent *entry) bool {
	return !r.closed && r.entries[ent.owner] == ent
}

// visibleLocked tells whether the responder answers a record on an
// interface. An A record is visible on the interface whose subnet holds
// its address. Other records are visible where one of the addresses of
// the owner is.
func (r *Responder) visibleLocked(ent *entry, rec *record, ifi iface) bool {
	if rec.addr.IsValid() {
		return ifi.contains(rec.addr)
	}

	return slices.ContainsFunc(ent.set.Addresses, ifi.contains)
}

// recordsOnLocked returns the records of an entry that keep accepts and
// that are visible on an interface.
func (r *Responder) recordsOnLocked(ent *entry, ifi iface, keep func(*record) bool) []dns.RR {
	var result []dns.RR

	for _, rec := range ent.records {
		if keep(rec) && r.visibleLocked(ent, rec, ifi) {
			result = append(result, rec.res)
		}
	}

	return result
}

// multicastIfacesLocked returns the interfaces that can send multicast.
func (r *Responder) multicastIfacesLocked() []iface {
	var result []iface

	for _, ifi := range r.ifaces {
		if ifi.multicast {
			result = append(result, ifi)
		}
	}

	return result
}

// refreshInterfacesLocked reads the interfaces of the host and joins the
// mDNS group on the interfaces that hold a published address. It leaves
// the group on the other interfaces, because the kernel allows only a
// few group memberships per socket.
func (r *Responder) refreshInterfacesLocked() {
	interfaces, err := r.listInterfaces()
	if err != nil {
		log.Logf(log.PriErr, "listing network interfaces: %v", err)

		return
	}

	r.ifaces = make(map[int]iface, len(interfaces))
	r.hostAddrs = make(map[netip.Addr]bool)

	for _, ifi := range interfaces {
		r.ifaces[ifi.index] = ifi

		for _, prefix := range ifi.prefixes {
			r.hostAddrs[prefix.Addr()] = true
		}
	}

	needed := make(map[int]bool)

	for _, ifi := range r.multicastIfacesLocked() {
		for _, ent := range r.entries {
			if slices.ContainsFunc(ent.set.Addresses, ifi.contains) {
				needed[ifi.index] = true
			}
		}
	}

	r.updateMembershipsLocked(needed)
}

// updateMembershipsLocked joins the mDNS group on the needed interfaces
// and leaves it on the others.
func (r *Responder) updateMembershipsLocked(needed map[int]bool) {
	for index := range r.joined {
		if needed[index] {
			continue
		}

		delete(r.joined, index)

		err := r.conn.leave(index)
		if err != nil {
			log.Logf(log.PriDebug, "%v", err)
		}
	}

	for index := range needed {
		if r.joined[index] {
			continue
		}

		err := r.conn.join(index)
		if err != nil {
			// Log a failure once per interface, because the
			// responder tries again on each refresh.
			priority := log.PriWarning
			if r.joinFailed[index] {
				priority = log.PriDebug
			}

			r.joinFailed[index] = true

			log.Logf(priority, "%v", err)

			continue
		}

		r.joined[index] = true
		delete(r.joinFailed, index)

		log.Logf(log.PriDebug, "joined mDNS group on %s", r.ifaces[index].name)
	}
}

func (r *Responder) rescanLoop() {
	ticker := time.NewTicker(r.timing.rescan)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.mutex.Lock()

			if !r.closed {
				r.refreshInterfacesLocked()
			}

			r.mutex.Unlock()
		}
	}
}

func (r *Responder) readLoop() {
	for {
		received, err := r.conn.readPacket()
		if err != nil {
			if r.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}

			log.Logf(log.PriErr, "reading mDNS packet: %v", err)

			continue
		}

		r.handlePacket(received)
	}
}

func (r *Responder) handlePacket(received packet) {
	msg := &dns.Msg{Data: received.data}

	err := msg.Unpack()
	if err != nil {
		return
	}

	if msg.Opcode != dns.OpcodeQuery || msg.Rcode != dns.RcodeSuccess {
		return
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.closed {
		return
	}

	if msg.Response {
		r.handleResponseLocked(msg, received)

		return
	}

	r.handleQueryLocked(msg, received)
}

// sleep waits for a duration. It returns false if the responder stops.
func (r *Responder) sleep(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-r.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sendResponseLocked sends a response with answers and additional
// records. It splits the answers over more packets when they do not fit
// in one.
func (r *Responder) sendResponseLocked(ifIndex int, dst netip.AddrPort, answers []dns.RR, extra []dns.RR) {
	for _, msg := range buildResponses(answers, extra) {
		r.sendLocked(ifIndex, dst, msg)
	}
}

func (r *Responder) sendLocked(ifIndex int, dst netip.AddrPort, msg *dns.Msg) {
	err := msg.Pack()
	if err != nil {
		log.Logf(log.PriErr, "packing mDNS message: %v", err)

		return
	}

	err = r.conn.writePacket(msg.Data, ifIndex, dst)
	if err != nil {
		log.Logf(log.PriWarning, "sending mDNS message on %s: %v", r.ifaces[ifIndex].name, err)
	}

	if dst == groupDst() {
		now := time.Now()

		for _, res := range msg.Answer {
			r.lastSent[sentKey{ifIndex: ifIndex, key: key(res)}] = now
		}
	}
}

// maxResponse is the largest response that ldddns sends. It fits in the
// 1500 byte MTU of a Docker bridge.
const maxResponse = 1400

func newResponse() *dns.Msg {
	msg := new(dns.Msg)
	msg.Response = true
	msg.Authoritative = true

	return msg
}

// buildResponses puts answers and additional records into responses
// that fit in maxResponse. Additional records that do not fit are left
// out.
func buildResponses(answers []dns.RR, extra []dns.RR) []*dns.Msg {
	if len(answers) == 0 {
		return nil
	}

	msgs := []*dns.Msg{newResponse()}

	for _, res := range answers {
		last := msgs[len(msgs)-1]
		last.Answer = append(last.Answer, res)

		if last.Len() > maxResponse && len(last.Answer) > 1 {
			last.Answer = last.Answer[:len(last.Answer)-1]

			next := newResponse()
			next.Answer = []dns.RR{res}
			msgs = append(msgs, next)
		}
	}

	last := msgs[len(msgs)-1]

	for _, res := range extra {
		last.Extra = append(last.Extra, res)

		if last.Len() > maxResponse {
			last.Extra = last.Extra[:len(last.Extra)-1]

			break
		}
	}

	return msgs
}
