//go:build !darwin

package peercred

import "net/netip"

func lookupOwnerUID(local, remote netip.AddrPort) (uint32, error) {
	return 0, ErrUnsupported
}
