package mdns

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"math"
	"slices"
	"strconv"
	"strings"

	"codeberg.org/miekg/dns"
)

// rrType returns the type of a record or a question.
func rrType(res dns.RR) uint16 {
	return dns.RRToType(res)
}

// rrClass returns the class of a record without the cache-flush bit.
func rrClass(res dns.RR) uint16 {
	return res.Header().Class &^ cacheFlush
}

func lowerName(res dns.RR) string {
	return strings.ToLower(res.Header().Name)
}

// key identifies a record by name, type, class and data. It ignores the
// TTL and the cache-flush bit.
func key(res dns.RR) string {
	return lowerName(res) + " " +
		strconv.Itoa(int(rrType(res))) + " " +
		strconv.Itoa(int(rrClass(res))) + " " +
		string(rdataBytes(res))
}

// rdataBytes returns the uncompressed wire format of the data of a
// record. Types that ldddns does not publish fall back to the text
// format of the data.
func rdataBytes(res dns.RR) []byte {
	switch res := res.(type) {
	case *dns.A:
		addr := res.Addr.As4()

		return addr[:]
	case *dns.AAAA:
		addr := res.Addr.As16()

		return addr[:]
	case *dns.PTR:
		return nameBytes(res.Ptr)
	case *dns.SRV:
		buf := binary.BigEndian.AppendUint16(nil, res.Priority)
		buf = binary.BigEndian.AppendUint16(buf, res.Weight)
		buf = binary.BigEndian.AppendUint16(buf, res.Port)

		return append(buf, nameBytes(res.Target)...)
	case *dns.TXT:
		var buf []byte

		for _, txt := range res.Txt {
			buf = appendString(buf, txt)
		}

		return buf
	}

	data := res.Data()
	if data == nil {
		return nil
	}

	return []byte(data.String())
}

// nameBytes returns a name in uncompressed wire format.
func nameBytes(name string) []byte {
	var buf []byte

	for label := range strings.SplitSeq(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}

		buf = appendString(buf, label)
	}

	return append(buf, 0)
}

// appendString appends a string with a length byte. A string longer
// than 255 bytes is cut, because a length byte cannot hold more.
func appendString(buf []byte, str string) []byte {
	str = str[:min(len(str), math.MaxUint8)]

	return append(append(buf, byte(len(str))), str...) //nolint:gosec // The length is at most 255.
}

// compareRecords orders records for the probe tie-break of RFC 6762
// section 8.2: first by class, then by type, then by data.
func compareRecords(a, b dns.RR) int {
	return cmp.Or(
		cmp.Compare(rrClass(a), rrClass(b)),
		cmp.Compare(rrType(a), rrType(b)),
		bytes.Compare(rdataBytes(a), rdataBytes(b)),
	)
}

// compareRecordSets compares two sets of records as RFC 6762 section
// 8.2 says. It returns a negative number if ours loses.
func compareRecordSets(ours, theirs []dns.RR) int {
	ours = slices.SortedFunc(slices.Values(ours), compareRecords)
	theirs = slices.SortedFunc(slices.Values(theirs), compareRecords)

	for i := range min(len(ours), len(theirs)) {
		if c := compareRecords(ours[i], theirs[i]); c != 0 {
			return c
		}
	}

	return cmp.Compare(len(ours), len(theirs))
}
