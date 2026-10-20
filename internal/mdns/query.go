package mdns

import (
	"iter"
	"slices"
	"time"

	"codeberg.org/miekg/dns"
)

const (
	// legacyTTL is the maximum TTL in a legacy unicast response (RFC
	// 6762 section 6.7).
	legacyTTL = 10
)

// answer is a record in a response.
type answer struct {
	res    dns.RR
	shared bool
}

// visibility tells whether a record can go in a response.
type visibility func(ent *entry, rec *record) bool

// handleQueryLocked answers a query (RFC 6762 section 6).
func (r *Responder) handleQueryLocked(msg *dns.Msg, received packet) {
	isProbe := len(msg.Ns) > 0
	if isProbe {
		r.tieBreakLocked(msg, received.ifIndex)
	}

	// A query from a port other than 5353 is a legacy unicast query
	// (section 6.7). It gets a unicast response.
	legacy := received.src.Port() != port
	visible := r.visibilityLocked(received, legacy)

	answers := r.answersLocked(msg.Question, visible)
	answers = suppressKnownAnswers(answers, msg.Answer)

	if !legacy {
		limit := r.timing.rateLimit
		if isProbe {
			limit = r.timing.probeRateLimit
		}

		answers = r.rateLimitLocked(answers, received.ifIndex, limit)
	}

	if len(answers) == 0 {
		return
	}

	extra := r.additionalLocked(answers, visible)

	if legacy {
		r.sendLegacyLocked(msg, received, answers, extra)

		return
	}

	r.sendMulticastLocked(received.ifIndex, answers, extra)
}

// visibilityLocked returns the visibility of records for a query.
func (r *Responder) visibilityLocked(received packet, legacy bool) visibility {
	// A unicast response goes only to the querier. If the querier is
	// this host, it can see all records.
	if legacy && r.hostAddrs[received.src.Addr()] {
		return func(*entry, *record) bool { return true }
	}

	ifi, ok := r.ifaces[received.ifIndex]
	if !ok || !ifi.multicast {
		return func(*entry, *record) bool { return false }
	}

	return func(ent *entry, rec *record) bool { return r.visibleLocked(ent, rec, ifi) }
}

// sendMulticastLocked sends a multicast response. The responder cannot
// send a unicast response to port 5353, because the kernel can give it
// to another socket on the port. A multicast response is always allowed
// (section 5.4).
func (r *Responder) sendMulticastLocked(ifIndex int, answers []answer, extra []dns.RR) {
	answerRRs := rrs(answers)

	if !slices.ContainsFunc(answers, func(ans answer) bool { return ans.shared }) {
		r.sendResponseLocked(ifIndex, groupDst(), answerRRs, extra)

		return
	}

	// A response with shared records waits a random time, so that the
	// responses of more hosts do not collide (section 6.3).
	r.wg.Go(func() {
		if !r.sleep(r.timing.sharedDelay()) {
			return
		}

		r.mutex.Lock()
		defer r.mutex.Unlock()

		if !r.closed {
			r.sendResponseLocked(ifIndex, groupDst(), answerRRs, extra)
		}
	})
}

// liveRecordsLocked returns the active and visible records of all
// entries.
func (r *Responder) liveRecordsLocked(visible visibility) iter.Seq[*record] {
	return func(yield func(*record) bool) {
		for _, ent := range r.entries {
			for _, rec := range ent.records {
				if ent.active(rec) && visible(ent, rec) && !yield(rec) {
					return
				}
			}
		}
	}
}

// answersLocked returns the active records that answer the questions.
func (r *Responder) answersLocked(questions []dns.RR, visible visibility) []answer {
	var answers []answer

	seen := make(map[string]bool)

	for _, question := range questions {
		for _, ans := range r.answerQuestionLocked(question, visible) {
			if !seen[key(ans.res)] {
				seen[key(ans.res)] = true
				answers = append(answers, ans)
			}
		}
	}

	return answers
}

// answerQuestionLocked returns the active records that answer one
// question. For a name with unique records but no record of the asked
// type, the answer is an NSEC record (section 6.1).
func (r *Responder) answerQuestionLocked(question dns.RR, visible visibility) []answer {
	class := rrClass(question)
	if class != dns.ClassINET && class != dns.ClassANY {
		return nil
	}

	name := lowerName(question)
	qtype := rrType(question)

	var (
		answers []answer
		types   []uint16
	)

	for rec := range r.liveRecordsLocked(visible) {
		if lowerName(rec.res) != name {
			continue
		}

		if rec.unique {
			types = append(types, rrType(rec.res))
		}

		if qtype == dns.TypeANY || qtype == rrType(rec.res) {
			answers = append(answers, answer{res: rec.res, shared: !rec.unique})
		}
	}

	if len(answers) == 0 && len(types) > 0 {
		return []answer{{res: nsec(question.Header().Name, types), shared: false}}
	}

	return answers
}

// nsec returns an NSEC record that lists the types a name has.
func nsec(name string, types []uint16) *dns.NSEC {
	types = slices.Compact(slices.Sorted(slices.Values(types)))

	return &dns.NSEC{
		Hdr:        dns.Header{Name: name, TTL: hostTTL, Class: dns.ClassINET | cacheFlush},
		NextDomain: name, TypeBitMap: types,
	}
}

// suppressKnownAnswers removes the answers that the querier already
// knows with at least half the TTL (RFC 6762 section 7.1).
func suppressKnownAnswers(answers []answer, known []dns.RR) []answer {
	return slices.DeleteFunc(answers, func(ans answer) bool {
		for _, res := range known {
			if key(res) == key(ans.res) && res.Header().TTL >= ans.res.Header().TTL/2 {
				return true
			}
		}

		return false
	})
}

// rateLimitLocked removes the answers that the responder multicast on
// the interface less than limit ago.
func (r *Responder) rateLimitLocked(answers []answer, ifIndex int, limit time.Duration) []answer {
	now := time.Now()

	for k, sent := range r.lastSent {
		if now.Sub(sent) >= r.timing.rateLimit {
			delete(r.lastSent, k)
		}
	}

	return slices.DeleteFunc(answers, func(ans answer) bool {
		sent, ok := r.lastSent[sentKey{ifIndex: ifIndex, key: key(ans.res)}]

		return ok && now.Sub(sent) < limit
	})
}

// additionalLocked returns the additional records for the answers
// (RFC 6763 section 12): SRV, TXT and A records for a PTR answer, and
// A records for an SRV answer.
func (r *Responder) additionalLocked(answers []answer, visible visibility) []dns.RR {
	seen := make(map[string]bool)

	for _, ans := range answers {
		seen[key(ans.res)] = true
	}

	var extra []dns.RR

	add := func(name string, types ...uint16) {
		for rec := range r.liveRecordsLocked(visible) {
			if dns.EqualName(rec.res.Header().Name, name) && slices.Contains(types, rrType(rec.res)) && !seen[key(rec.res)] {
				seen[key(rec.res)] = true
				extra = append(extra, rec.res)
			}
		}
	}

	for _, ans := range answers {
		if ptr, ok := ans.res.(*dns.PTR); ok {
			add(ptr.Ptr, dns.TypeSRV, dns.TypeTXT)
		}
	}

	for _, res := range slices.Concat(rrs(answers), extra) {
		if srv, ok := res.(*dns.SRV); ok {
			add(srv.Target, dns.TypeA)
		}
	}

	return extra
}

// sendLegacyLocked sends a unicast response to a legacy query. It
// repeats the query ID and questions, caps the TTL and clears the
// cache-flush bit (RFC 6762 section 6.7).
func (r *Responder) sendLegacyLocked(query *dns.Msg, received packet, answers []answer, extra []dns.RR) {
	msg := newResponse()
	msg.ID = query.ID

	for _, question := range query.Question {
		question = question.Clone()
		question.Header().Class = rrClass(question)
		msg.Question = append(msg.Question, question)
	}

	legacyRR := func(res dns.RR) dns.RR {
		res = res.Clone()
		res.Header().Class = rrClass(res)
		res.Header().TTL = min(res.Header().TTL, legacyTTL)

		return res
	}

	for _, ans := range answers {
		msg.Answer = append(msg.Answer, legacyRR(ans.res))
	}

	for _, res := range extra {
		msg.Extra = append(msg.Extra, legacyRR(res))

		if msg.Len() > maxResponse {
			msg.Extra = msg.Extra[:len(msg.Extra)-1]

			break
		}
	}

	r.sendLocked(received.ifIndex, received.src, msg)
}

func rrs(answers []answer) []dns.RR {
	result := make([]dns.RR, 0, len(answers))

	for _, ans := range answers {
		result = append(result, ans.res)
	}

	return result
}
