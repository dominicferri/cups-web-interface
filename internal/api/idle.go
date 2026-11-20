package api

import "time"

func (s *Server) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

// WaitIdle blocks until no browser is using the server: no event stream is
// open and no /api/ request arrived for grace. Before the first request it
// waits startWindow instead, so a browser that is still opening has time to
// connect. grace must outlast a page reload and an EventSource reconnect.
func (s *Server) WaitIdle(grace, startWindow, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for range ticker.C {
		if s.events.SubscriberCount() > 0 {
			continue
		}
		last, limit := s.started, startWindow
		if n := s.lastActivity.Load(); n != 0 {
			last, limit = time.Unix(0, n), grace
		}
		if time.Since(last) >= limit {
			return
		}
	}
}
