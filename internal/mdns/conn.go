package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

const (
	port = 5353
	// maxPacket is the largest packet ldddns reads.
	maxPacket = 9000
	// multicastTTL is the IP TTL of mDNS packets (RFC 6762 section 11).
	multicastTTL = 255
)

// groupAddr returns the mDNS multicast address.
func groupAddr() netip.Addr {
	return netip.AddrFrom4([4]byte{224, 0, 0, 251})
}

// groupDst returns the destination of a multicast packet.
func groupDst() netip.AddrPort {
	return netip.AddrPortFrom(groupAddr(), port)
}

// packet is a received packet.
type packet struct {
	data    []byte
	ifIndex int
	src     netip.AddrPort
}

// transport sends and receives mDNS packets.
type transport interface {
	readPacket() (packet, error)
	writePacket(data []byte, ifIndex int, dst netip.AddrPort) error
	join(ifIndex int) error
	leave(ifIndex int) error
	close() error
}

// udpTransport is a transport on a UDP socket on port 5353.
type udpTransport struct {
	conn *ipv4.PacketConn
}

// listen opens the mDNS socket. Avahi and other mDNS responders on the
// host bind the same port, so the socket sets SO_REUSEADDR and
// SO_REUSEPORT.
func listen(ctx context.Context) (*udpTransport, error) {
	config := net.ListenConfig{Control: reusePort}

	conn, err := config.ListenPacket(ctx, "udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listening on mDNS port: %w", err)
	}

	packetConn := ipv4.NewPacketConn(conn)

	err = errors.Join(
		packetConn.SetMulticastTTL(multicastTTL),
		packetConn.SetMulticastLoopback(true),
		packetConn.SetControlMessage(ipv4.FlagInterface|ipv4.FlagDst, true),
	)
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("setting socket options: %w", err)
	}

	return &udpTransport{conn: packetConn}, nil
}

func reusePort(_, _ string, rawConn syscall.RawConn) error {
	var sockErr error

	err := rawConn.Control(func(fd uintptr) {
		sockErr = errors.Join(
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1),
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1),
		)
	})
	if err != nil {
		return fmt.Errorf("controlling socket: %w", err)
	}

	return sockErr
}

func (t *udpTransport) readPacket() (packet, error) {
	buf := make([]byte, maxPacket)

	n, controlMessage, src, err := t.conn.ReadFrom(buf)
	if err != nil {
		return packet{}, fmt.Errorf("reading packet: %w", err)
	}

	result := packet{data: buf[:n], ifIndex: 0, src: netip.AddrPort{}}

	if controlMessage != nil {
		result.ifIndex = controlMessage.IfIndex
	}

	if udpAddr, ok := src.(*net.UDPAddr); ok {
		result.src = udpAddr.AddrPort()
		result.src = netip.AddrPortFrom(result.src.Addr().Unmap(), result.src.Port())
	}

	return result, nil
}

func (t *udpTransport) writePacket(data []byte, ifIndex int, dst netip.AddrPort) error {
	var controlMessage *ipv4.ControlMessage

	if ifIndex > 0 {
		controlMessage = &ipv4.ControlMessage{IfIndex: ifIndex}
	}

	_, err := t.conn.WriteTo(data, controlMessage, net.UDPAddrFromAddrPort(dst))
	if err != nil {
		return fmt.Errorf("writing packet: %w", err)
	}

	return nil
}

func (t *udpTransport) join(ifIndex int) error {
	ifi, err := net.InterfaceByIndex(ifIndex)
	if err != nil {
		return fmt.Errorf("looking up interface %d: %w", ifIndex, err)
	}

	err = t.conn.JoinGroup(ifi, &net.UDPAddr{IP: groupAddr().AsSlice()})
	if err != nil {
		return fmt.Errorf("joining mDNS group on %s: %w", ifi.Name, err)
	}

	return nil
}

func (t *udpTransport) leave(ifIndex int) error {
	ifi, err := net.InterfaceByIndex(ifIndex)
	if err != nil {
		return fmt.Errorf("looking up interface %d: %w", ifIndex, err)
	}

	err = t.conn.LeaveGroup(ifi, &net.UDPAddr{IP: groupAddr().AsSlice()})
	if err != nil {
		return fmt.Errorf("leaving mDNS group on %s: %w", ifi.Name, err)
	}

	return nil
}

func (t *udpTransport) close() error {
	err := t.conn.Close()
	if err != nil {
		return fmt.Errorf("closing socket: %w", err)
	}

	return nil
}

// iface is a network interface of the host.
type iface struct {
	index     int
	name      string
	multicast bool
	prefixes  []netip.Prefix
}

func (i iface) contains(addr netip.Addr) bool {
	for _, prefix := range i.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// systemInterfaces returns the up interfaces of the host with their
// IPv4 prefixes.
func systemInterfaces() ([]iface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing interfaces: %w", err)
	}

	var result []iface

	for _, netInterface := range interfaces {
		if netInterface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := netInterface.Addrs()
		if err != nil {
			continue
		}

		var prefixes []netip.Prefix

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			prefix, err := netip.ParsePrefix(ipNet.String())
			if err != nil || !prefix.Addr().Is4() {
				continue
			}

			prefixes = append(prefixes, prefix)
		}

		if len(prefixes) == 0 {
			continue
		}

		result = append(result, iface{
			index:     netInterface.Index,
			name:      netInterface.Name,
			multicast: netInterface.Flags&net.FlagMulticast != 0,
			prefixes:  prefixes,
		})
	}

	return result, nil
}
