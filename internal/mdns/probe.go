package mdns

import (
	"math/rand/v2"
	"slices"

	"codeberg.org/miekg/dns"
	"ldddns.arnested.dk/internal/log"
)

// startProbeLocked starts a probe goroutine for the names of an entry
// that are in the probing state.
func (r *Responder) startProbeLocked(ent *entry) {
	if ent.probing || len(ent.namesIn(stateProbing)) == 0 {
		return
	}

	ent.probing = true

	r.wg.Go(func() { r.probe(ent) })
}

// probe probes the names of an entry (RFC 6762 section 8.1). Names
// without a conflict go live and get announced (section 8.3).
func (r *Responder) probe(ent *entry) {
	// A random delay before the first probe (section 8.1).
	if !r.sleep(rand.N(r.timing.probe)) { //nolint:gosec // Jitter only.
		return
	}

	for {
		r.mutex.Lock()

		if !r.current(ent) {
			r.mutex.Unlock()

			return
		}

		names := ent.namesIn(stateProbing)
		if len(names) == 0 {
			ent.probing = false
			r.mutex.Unlock()

			return
		}

		r.mutex.Unlock()

		if !r.probeNames(ent, names) {
			return
		}
	}
}

// probeNames sends the probes for names and announces the names that
// survive. It returns false if the entry is gone or the responder
// stops.
func (r *Responder) probeNames(ent *entry, names []string) bool {
	for range probeCount {
		r.mutex.Lock()

		if !r.current(ent) {
			r.mutex.Unlock()

			return false
		}

		if ent.lostTieBreak {
			ent.lostTieBreak = false
			r.mutex.Unlock()

			// The other host defends the name after it wins, so the
			// next round finds the conflict (section 8.2).
			return r.sleep(r.timing.tieBreak)
		}

		r.sendProbeLocked(ent, names)
		r.mutex.Unlock()

		if !r.sleep(r.timing.probe) {
			return false
		}
	}

	r.mutex.Lock()

	if !r.current(ent) {
		r.mutex.Unlock()

		return false
	}

	if ent.lostTieBreak {
		ent.lostTieBreak = false
		r.mutex.Unlock()

		return r.sleep(r.timing.tieBreak)
	}

	var live []string

	for _, name := range names {
		if ent.states[name] == stateProbing {
			ent.states[name] = stateLive
			live = append(live, name)

			log.Logf(log.PriInfo, "mDNS name %s of container %s is live", name, ent.owner)
		}
	}

	r.mutex.Unlock()

	return r.announce(ent, live)
}

// sendProbeLocked sends one probe for names on each interface where the
// entry has records. The question asks for a unicast response, and the
// authority section holds the proposed records.
func (r *Responder) sendProbeLocked(ent *entry, names []string) {
	for _, ifi := range r.multicastIfacesLocked() {
		msg := new(dns.Msg)

		for _, name := range names {
			proposed := r.recordsOnLocked(ent, ifi, func(rec *record) bool {
				return rec.unique && rec.claim == name && lowerName(rec.res) == name
			})
			if len(proposed) == 0 {
				continue
			}

			msg.Question = append(msg.Question, &dns.ANY{
				Hdr: dns.Header{Name: proposed[0].Header().Name, TTL: 0, Class: dns.ClassINET | cacheFlush},
			})
			msg.Ns = append(msg.Ns, proposed...)
		}

		if len(msg.Question) > 0 {
			r.sendLocked(ifi.index, groupDst(), msg)
		}
	}
}

// announce sends the live records of names twice, one interval apart.
func (r *Responder) announce(ent *entry, names []string) bool {
	if len(names) == 0 {
		return true
	}

	for i := range 2 {
		if i > 0 && !r.sleep(r.timing.announce) {
			return false
		}

		r.mutex.Lock()

		if !r.current(ent) {
			r.mutex.Unlock()

			return false
		}

		for _, ifi := range r.multicastIfacesLocked() {
			answers := r.recordsOnLocked(ent, ifi, func(rec *record) bool {
				return slices.Contains(names, rec.claim) && ent.active(rec)
			})

			r.sendResponseLocked(ifi.index, groupDst(), answers, nil)
		}

		r.mutex.Unlock()
	}

	return true
}

// tieBreakLocked handles a probe from another host for a name that the
// responder probes too (RFC 6762 section 8.2).
func (r *Responder) tieBreakLocked(msg *dns.Msg, ifIndex int) {
	theirs := make(map[string][]dns.RR)

	for _, res := range msg.Ns {
		name := lowerName(res)
		theirs[name] = append(theirs[name], res)
	}

	for name, their := range theirs {
		for _, ent := range r.entries {
			if ent.states[name] == stateProbing && r.losesTieBreakLocked(ent, name, their, ifIndex) {
				log.Logf(log.PriInfo, "lost the mDNS probe tie-break for %s of container %s", name, ent.owner)

				ent.lostTieBreak = true
			}
		}
	}
}

// losesTieBreakLocked compares the records of a probe from another host
// with the records of an entry for the same name.
func (r *Responder) losesTieBreakLocked(ent *entry, name string, theirs []dns.RR, ifIndex int) bool {
	ifi, known := r.ifaces[ifIndex]
	ourKeys := make(map[string]bool)

	var ours []dns.RR

	for _, rec := range ent.records {
		if !rec.unique || lowerName(rec.res) != name {
			continue
		}

		ourKeys[key(rec.res)] = true

		if !known || r.visibleLocked(ent, rec, ifi) {
			ours = append(ours, rec.res)
		}
	}

	// A probe that holds only our records is our own probe, looped
	// back.
	if subset(theirs, ourKeys) {
		return false
	}

	return compareRecordSets(ours, theirs) < 0
}

func subset(records []dns.RR, keys map[string]bool) bool {
	for _, res := range records {
		if !keys[key(res)] {
			return false
		}
	}

	return true
}

// handleResponseLocked looks for conflicts in a response (RFC 6762
// section 9). During probing, any record with the name is a conflict.
// For a live name, a record of the same type with other data is a
// conflict, and the responder probes the name again.
func (r *Responder) handleResponseLocked(msg *dns.Msg, received packet) {
	// Responses from other ports are not mDNS responses (section 6).
	if received.src.Port() != port {
		return
	}

	for _, res := range slices.Concat(msg.Answer, msg.Ns, msg.Extra) {
		// A goodbye is no claim on the name.
		if res.Header().TTL == 0 {
			continue
		}

		name := lowerName(res)

		for _, ent := range r.entries {
			state, ok := ent.states[name]
			if !ok || state == stateConflict || !r.conflicts(ent, res) {
				continue
			}

			if state == stateProbing {
				log.Logf(log.PriWarning, "mDNS name conflict for %s of container %s", name, ent.owner)

				ent.states[name] = stateConflict

				continue
			}

			log.Logf(log.PriNotice, "mDNS name conflict for live name %s of container %s, probing again", name, ent.owner)

			ent.states[name] = stateProbing
			r.startProbeLocked(ent)
		}
	}
}

// conflicts tells whether a record from another host conflicts with the
// unique records of an entry.
func (r *Responder) conflicts(ent *entry, res dns.RR) bool {
	name := lowerName(res)
	probing := ent.states[name] == stateProbing
	sameType := false

	for _, rec := range ent.records {
		if !rec.unique || lowerName(rec.res) != name {
			continue
		}

		if key(rec.res) == key(res) {
			return false
		}

		if rrType(rec.res) == rrType(res) && rrClass(rec.res) == rrClass(res) {
			sameType = true
		}
	}

	return probing || sameType
}
