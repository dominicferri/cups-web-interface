//go:build darwin

package peercred

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"syscall"
)

// The kernel publishes its TCP connection table through the
// net.inet.tcp.pcblist_n sysctl, readable without privileges. The table is a
// header followed by a group of records per connection. Each record starts with
// a 32-bit length and a 32-bit kind, and is padded to 8 bytes. Within a group
// the connection record (addresses and ports) comes first and the socket record
// (owning user ID) follows it.
//
// These layouts come from the XNU sources (struct xinpcb_n and xsocket_n) and
// are not a public API, so SelfTest checks them against a real connection.
const (
	pcbListSysctl = "net.inet.tcp.pcblist_n"

	kindSocket = 0x001 // struct xsocket_n
	kindInPCB  = 0x010 // struct xinpcb_n

	// struct xinpcb_n
	inpForeignPortOff = 16 // network byte order
	inpLocalPortOff   = 18 // network byte order
	inpVFlagOff       = 44
	inpForeignAddrOff = 48 // 16 bytes; an IPv4 address occupies the last 4
	inpLocalAddrOff   = 64 // 16 bytes; an IPv4 address occupies the last 4
	inpMinLen         = 80

	inpVFlagIPv4 = 0x1
	inpVFlagIPv6 = 0x2

	// struct xsocket_n
	soUIDOff = 64
	soMinLen = 68
)

func lookupOwnerUID(local, remote netip.AddrPort) (uint32, error) {
	var table string
	var err error
	// The table can grow between the kernel's size probe and the read.
	for attempt := 0; attempt < 3; attempt++ {
		table, err = syscall.Sysctl(pcbListSysctl)
		if !errors.Is(err, syscall.ENOMEM) {
			break
		}
	}
	if err != nil {
		return 0, fmt.Errorf("peercred: reading %s: %w", pcbListSysctl, err)
	}
	return findOwnerUID([]byte(table), local, remote)
}

// findOwnerUID scans a pcblist_n table for the peer's socket. From the peer's
// point of view its own address is the local one and the server's is foreign.
func findOwnerUID(buf []byte, server, peer netip.AddrPort) (uint32, error) {
	le := binary.LittleEndian
	if len(buf) < 4 {
		return 0, errors.New("peercred: connection table is too short")
	}
	off := int(le.Uint32(buf))
	if off < 8 {
		return 0, errors.New("peercred: connection table header is malformed")
	}

	var (
		matched bool // the current group's connection record is the peer
		found   bool
		uid     uint32
	)
	for off+8 <= len(buf) {
		recLen := int(le.Uint32(buf[off:]))
		kind := le.Uint32(buf[off+4:])
		if recLen < 8 || off+recLen > len(buf) {
			break
		}
		rec := buf[off : off+recLen]

		switch kind {
		case kindInPCB:
			if recLen < inpMinLen {
				return 0, errors.New("peercred: unexpected connection record size")
			}
			matched = inpMatches(rec, server, peer)
		case kindSocket:
			if recLen < soMinLen {
				return 0, errors.New("peercred: unexpected socket record size")
			}
			if matched {
				owner := le.Uint32(rec[soUIDOff:])
				if found && owner != uid {
					return 0, errors.New("peercred: peer matches sockets of more than one user")
				}
				found, uid = true, owner
				matched = false
			}
		}
		off += (recLen + 7) &^ 7
	}

	if !found {
		return 0, ErrNotFound
	}
	return uid, nil
}

func inpMatches(rec []byte, server, peer netip.AddrPort) bool {
	if binary.BigEndian.Uint16(rec[inpLocalPortOff:]) != peer.Port() ||
		binary.BigEndian.Uint16(rec[inpForeignPortOff:]) != server.Port() {
		return false
	}
	vflag := rec[inpVFlagOff]
	return inpAddr(rec[inpLocalAddrOff:inpLocalAddrOff+16], vflag) == peer.Addr() &&
		inpAddr(rec[inpForeignAddrOff:inpForeignAddrOff+16], vflag) == server.Addr()
}

func inpAddr(b []byte, vflag byte) netip.Addr {
	if vflag&inpVFlagIPv4 != 0 && vflag&inpVFlagIPv6 == 0 {
		return netip.AddrFrom4([4]byte(b[12:16]))
	}
	return netip.AddrFrom16([16]byte(b)).Unmap()
}
