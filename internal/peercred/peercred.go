// Package peercred identifies which local user owns the other end of a TCP
// connection accepted by this process.
//
// The web interface listens on loopback TCP, which every account on the machine
// can reach. To avoid handing the session token to other users, the server only
// treats a connection as trusted when the socket at the other end belongs to the
// same user ID as the server itself.
package peercred

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
)

// ErrNotFound means no local socket matches the connection: the peer is on
// another machine, or has already closed its end.
var ErrNotFound = errors.New("peercred: no local socket found for peer")

// ErrUnsupported means peer lookup is not implemented for this platform.
var ErrUnsupported = errors.New("peercred: peer lookup is not supported on this platform")

// OwnerUID returns the user ID owning the peer's socket for a TCP connection
// this process accepted. local is the server side of the connection and remote
// is the peer.
func OwnerUID(local, remote net.Addr) (uint32, error) {
	l, err := addrPort(local)
	if err != nil {
		return 0, err
	}
	r, err := addrPort(remote)
	if err != nil {
		return 0, err
	}
	return lookupOwnerUID(l, r)
}

// IsSameUser reports whether the peer's socket is owned by the user running
// this process. Any lookup failure is returned as an error and must be treated
// as "not the same user".
func IsSameUser(local, remote net.Addr) (bool, error) {
	uid, err := OwnerUID(local, remote)
	if err != nil {
		return false, err
	}
	return uid == uint32(os.Getuid()), nil
}

// SelfTest opens a loopback connection to a temporary listener and checks that
// its owner resolves to the current user. It guards against the operating
// system changing the layout of the data peer lookup relies on.
func SelfTest() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return err
	}
	defer client.Close()

	server, err := ln.Accept()
	if err != nil {
		return err
	}
	defer server.Close()

	uid, err := OwnerUID(server.LocalAddr(), server.RemoteAddr())
	if err != nil {
		return err
	}
	if uid != uint32(os.Getuid()) {
		return fmt.Errorf("peercred: self-test resolved uid %d, expected %d", uid, os.Getuid())
	}
	return nil
}

func addrPort(a net.Addr) (netip.AddrPort, error) {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp == nil {
		return netip.AddrPort{}, fmt.Errorf("peercred: not a TCP address: %v", a)
	}
	ap := tcp.AddrPort()
	if !ap.IsValid() {
		return netip.AddrPort{}, fmt.Errorf("peercred: invalid TCP address: %v", a)
	}
	// Compare IPv4 and IPv4-mapped IPv6 forms as equal, and ignore zones.
	return netip.AddrPortFrom(ap.Addr().Unmap().WithZone(""), ap.Port()), nil
}
