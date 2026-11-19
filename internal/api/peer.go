package api

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/PaperCutSoftware/cups-web-interface/internal/peercred"
)

// The server listens on loopback TCP, which every account on the machine can
// reach. A connection is "trusted" only when the socket at the other end is
// owned by the same user that runs this server. Trusted connections may read
// the API and be issued the session token without presenting it first, which
// is what lets a user open http://127.0.0.1:6310 directly in their browser.
// All other connections (other local users, other machines, or any connection
// whose owner cannot be determined) must already hold the token.

type peerContextKey struct{}

// peerState caches the ownership check for one TCP connection.
type peerState struct {
	once     sync.Once
	sameUser bool
	local    net.Addr
	remote   net.Addr
}

// ConnContext is an http.Server.ConnContext hook. It attaches per-connection
// state so the ownership check runs at most once per connection.
func (s *Server) ConnContext(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, peerContextKey{}, &peerState{
		local:  c.LocalAddr(),
		remote: c.RemoteAddr(),
	})
}

// connectionIsSameUser is the default trust check. It fails closed: without
// connection state, or on any lookup error, the peer is not trusted.
func connectionIsSameUser(r *http.Request) bool {
	st, ok := r.Context().Value(peerContextKey{}).(*peerState)
	if !ok || st == nil {
		return false
	}
	st.once.Do(func() {
		same, err := peercred.IsSameUser(st.local, st.remote)
		st.sameUser = err == nil && same
	})
	return st.sameUser
}

// isTrustedPeer reports whether the request arrived on a connection owned by
// the user running this server.
func (s *Server) isTrustedPeer(r *http.Request) bool {
	if s.peerIsSameUser == nil {
		return false
	}
	return s.peerIsSameUser(r)
}
