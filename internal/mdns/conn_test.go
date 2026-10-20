//nolint:testpackage // The tests use the unexported transport.
package mdns

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

func TestIfaceContains(t *testing.T) {
	t.Parallel()

	ifi := iface{
		index:     1,
		name:      "docker0",
		multicast: true,
		prefixes:  []netip.Prefix{netip.MustParsePrefix("172.17.0.1/16")},
	}

	if !ifi.contains(containerAddr) {
		t.Errorf("Expected %s in %v", containerAddr, ifi.prefixes)
	}

	if ifi.contains(lanPeer) {
		t.Errorf("Expected %s not in %v", lanPeer, ifi.prefixes)
	}
}

func TestSystemInterfaces(t *testing.T) {
	t.Parallel()

	interfaces, err := systemInterfaces()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	for _, ifi := range interfaces {
		if ifi.index <= 0 || ifi.name == "" || len(ifi.prefixes) == 0 {
			t.Errorf("Expected an index, a name and prefixes, got %+v", ifi)
		}

		for _, prefix := range ifi.prefixes {
			if !prefix.Addr().Is4() {
				t.Errorf("Expected only IPv4 prefixes, got %s", prefix)
			}
		}
	}
}

// listenOrSkip opens the mDNS socket. It skips the test if the host
// does not allow the socket.
func listenOrSkip(t *testing.T) *udpTransport {
	t.Helper()

	conn, err := listen(t.Context())
	if err != nil {
		t.Skipf("cannot open the mDNS socket: %v", err)
	}

	return conn
}

// The tests that bind port 5353 do not run in parallel, so that they do
// not get each other's packets.

//nolint:paralleltest // The test binds port 5353.
func TestUDPTransportReadsAndWrites(t *testing.T) {
	conn := listenOrSkip(t)
	received := readPackets(conn)

	msg := new(dns.Msg)
	msg.Question = []dns.RR{question("udp-test.local.", dns.TypeA)}

	err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}

	got := sendUntilReceived(t, conn, msg.Data, received)

	if !got.src.Addr().IsLoopback() || got.ifIndex == 0 || len(got.data) != len(msg.Data) {
		t.Errorf("Expected the packet from the loopback interface, got %+v", got)
	}

	err = conn.close()
	if err != nil {
		t.Errorf("Expected no close error, got %v", err)
	}

	waitForClosed(t, received)

	_, err = conn.readPacket()
	if !errors.Is(err, net.ErrClosed) {
		t.Errorf("Expected net.ErrClosed after close, got %v", err)
	}

	if conn.close() == nil {
		t.Error("Expected an error from a second close")
	}
}

// readPackets reads packets from a transport until it closes. Then it
// closes the returned channel.
func readPackets(conn *udpTransport) <-chan packet {
	received := make(chan packet, 1)

	go func() {
		defer close(received)

		for {
			got, err := conn.readPacket()
			if err != nil {
				return
			}

			select {
			case received <- got:
			default:
			}
		}
	}()

	return received
}

// sendUntilReceived sends data to the loopback address until the
// transport receives it. Other sockets on port 5353 can get a unicast
// packet, so it sends more than one.
func sendUntilReceived(t *testing.T, conn *udpTransport, data []byte, received <-chan packet) packet {
	t.Helper()

	deadline := time.After(2 * time.Second)

	for {
		err := conn.writePacket(data, 0, netip.MustParseAddrPort("127.0.0.1:5353"))
		if err != nil {
			t.Fatalf("Expected no write error, got %v", err)
		}

		select {
		case got := <-received:
			return got
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Skip("another socket on port 5353 got all packets")
		}
	}
}

// waitForClosed reads packets until the channel closes.
func waitForClosed(t *testing.T, received <-chan packet) {
	t.Helper()

	timeout := time.After(testTimeout)

	for {
		select {
		case _, open := <-received:
			if !open {
				return
			}
		case <-timeout:
			t.Fatal("Expected close to stop the read loop")
		}
	}
}

//nolint:paralleltest // The test binds port 5353.
func TestUDPTransportJoinAndLeave(t *testing.T) {
	conn := listenOrSkip(t)
	t.Cleanup(func() { _ = conn.close() })

	if conn.join(0) == nil || conn.leave(0) == nil {
		t.Error("Expected errors for an unknown interface")
	}

	interfaces, err := systemInterfaces()
	if err != nil {
		t.Fatal(err)
	}

	for _, ifi := range interfaces {
		if !ifi.multicast {
			continue
		}

		err := conn.join(ifi.index)
		if err != nil {
			t.Skipf("cannot join the mDNS group on %s: %v", ifi.name, err)
		}

		err = conn.leave(ifi.index)
		if err != nil {
			t.Errorf("Expected no error leaving the group on %s, got %v", ifi.name, err)
		}

		if conn.leave(ifi.index) == nil {
			t.Errorf("Expected an error leaving the group on %s twice", ifi.name)
		}

		return
	}

	t.Skip("no multicast interface")
}

//nolint:paralleltest // The test binds port 5353.
func TestNewAndClose(t *testing.T) {
	responder, err := New(t.Context())
	if err != nil {
		t.Skipf("cannot open the mDNS socket: %v", err)
	}

	err = responder.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
