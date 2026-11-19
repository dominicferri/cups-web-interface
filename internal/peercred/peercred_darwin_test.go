//go:build darwin

package peercred

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"testing"
)

func TestSelfTest(t *testing.T) {
	if err := SelfTest(); err != nil {
		t.Fatalf("SelfTest failed: %v", err)
	}
}

func TestOwnerUIDLiveConnections(t *testing.T) {
	for _, network := range []struct{ name, addr string }{
		{"ipv4", "127.0.0.1:0"},
		{"ipv6", "[::1]:0"},
	} {
		t.Run(network.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", network.addr)
			if err != nil {
				t.Skipf("cannot listen on %s: %v", network.addr, err)
			}
			defer ln.Close()
			client, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			server, err := ln.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()

			uid, err := OwnerUID(server.LocalAddr(), server.RemoteAddr())
			if err != nil {
				t.Fatalf("OwnerUID: %v", err)
			}
			if uid != uint32(os.Getuid()) {
				t.Fatalf("expected uid %d, got %d", os.Getuid(), uid)
			}
			same, err := IsSameUser(server.LocalAddr(), server.RemoteAddr())
			if err != nil || !same {
				t.Fatalf("IsSameUser = %v, %v; want true, nil", same, err)
			}
		})
	}
}

func TestOwnerUIDPeerNotLocal(t *testing.T) {
	// A peer address that has no socket on this machine (e.g. a LAN client).
	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 6310}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.55"), Port: 40000}
	if _, err := OwnerUID(local, remote); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if same, err := IsSameUser(local, remote); same || err == nil {
		t.Fatalf("IsSameUser = %v, %v; want false with an error", same, err)
	}
}

func TestOwnerUIDRejectsNonTCP(t *testing.T) {
	if _, err := OwnerUID(&net.UnixAddr{Name: "/tmp/x"}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}); err == nil {
		t.Fatal("expected an error for a non-TCP address")
	}
	if _, err := OwnerUID(nil, nil); err == nil {
		t.Fatal("expected an error for nil addresses")
	}
}

// buildTable assembles a synthetic pcblist_n table from (peer, server, uid) entries.
type entry struct {
	peer, server netip.AddrPort
	uid          uint32
}

func buildTable(entries ...entry) []byte {
	le := binary.LittleEndian
	buf := make([]byte, 24)
	le.PutUint32(buf, 24)
	for _, e := range entries {
		inp := make([]byte, 104)
		le.PutUint32(inp, 104)
		le.PutUint32(inp[4:], kindInPCB)
		binary.BigEndian.PutUint16(inp[inpForeignPortOff:], e.server.Port())
		binary.BigEndian.PutUint16(inp[inpLocalPortOff:], e.peer.Port())
		if e.peer.Addr().Is4() {
			inp[inpVFlagOff] = inpVFlagIPv4
			p, s := e.peer.Addr().As4(), e.server.Addr().As4()
			copy(inp[inpLocalAddrOff+12:], p[:])
			copy(inp[inpForeignAddrOff+12:], s[:])
		} else {
			inp[inpVFlagOff] = inpVFlagIPv6
			p, s := e.peer.Addr().As16(), e.server.Addr().As16()
			copy(inp[inpLocalAddrOff:], p[:])
			copy(inp[inpForeignAddrOff:], s[:])
		}
		buf = append(buf, inp...)

		so := make([]byte, 104)
		le.PutUint32(so, 104)
		le.PutUint32(so[4:], kindSocket)
		le.PutUint32(so[soUIDOff:], e.uid)
		buf = append(buf, so...)

		// An unrelated record kind with a length that needs padding to 8 bytes.
		other := make([]byte, 32)
		le.PutUint32(other, 28)
		le.PutUint32(other[4:], 0x002)
		buf = append(buf, other...)
	}
	return buf
}

func TestFindOwnerUIDSynthetic(t *testing.T) {
	server := netip.MustParseAddrPort("127.0.0.1:6310")
	peer := netip.MustParseAddrPort("127.0.0.1:50000")

	table := buildTable(
		// The server's own end of the connection, owned by the server user.
		entry{peer: server, server: peer, uid: 502},
		// Same ports but a different local address: must not match.
		entry{peer: netip.MustParseAddrPort("10.0.0.5:50000"), server: netip.MustParseAddrPort("10.0.0.9:6310"), uid: 502},
		// The peer itself, owned by another user.
		entry{peer: peer, server: server, uid: 503},
		// A neighbouring connection from the server user.
		entry{peer: netip.MustParseAddrPort("127.0.0.1:50001"), server: server, uid: 502},
	)

	uid, err := findOwnerUID(table, server, peer)
	if err != nil {
		t.Fatalf("findOwnerUID: %v", err)
	}
	if uid != 503 {
		t.Fatalf("expected the peer's uid 503, got %d", uid)
	}

	if _, err := findOwnerUID(table, server, netip.MustParseAddrPort("127.0.0.1:50999")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown peer, got %v", err)
	}

	v6server := netip.MustParseAddrPort("[::1]:6310")
	v6peer := netip.MustParseAddrPort("[::1]:50000")
	v6table := buildTable(entry{peer: v6peer, server: v6server, uid: 504})
	if uid, err := findOwnerUID(v6table, v6server, v6peer); err != nil || uid != 504 {
		t.Fatalf("ipv6: got uid %d, err %v; want 504", uid, err)
	}
}

func TestFindOwnerUIDMalformed(t *testing.T) {
	server := netip.MustParseAddrPort("127.0.0.1:6310")
	peer := netip.MustParseAddrPort("127.0.0.1:50000")

	for name, table := range map[string][]byte{
		"empty":          {},
		"short header":   {1, 2},
		"zero header":    make([]byte, 24),
		"header only":    buildTable(),
		"truncated":      buildTable(entry{peer: peer, server: server, uid: 502})[:24+60],
		"conflicting":    buildTable(entry{peer: peer, server: server, uid: 502}, entry{peer: peer, server: server, uid: 503}),
		"record overrun": append(buildTable(), 0xff, 0xff, 0, 0, 0x10, 0, 0, 0),
	} {
		if uid, err := findOwnerUID(table, server, peer); err == nil {
			t.Errorf("%s: expected an error, got uid %d", name, uid)
		}
	}
}
