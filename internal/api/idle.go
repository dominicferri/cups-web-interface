package api

import "time"

func (s *Server) touch() {
	s.lastActivity.Store(int64(time.Since(s.started)))
}

// WaitIdle blocks until no browser is using the server: no event stream is
// open and no request counted by touch arrived for grace. It never returns
// before startWindow has passed since the start, so a browser that is still
// opening has time to connect. grace must outlast a page reload and an
// EventSource reconnect.
func (s *Server) WaitIdle(grace, startWindow, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for range ticker.C {
		if s.events.SubscriberCount() > 0 {
			continue
		}
		// time.Since on s.started reads the monotonic clock, so a step of the
		// system clock does not shorten or stretch either period.
		sinceStart := time.Since(s.started)
		if sinceStart < startWindow {
			continue
		}
		if n := s.lastActivity.Load(); n != 0 && sinceStart-time.Duration(n) < grace {
			continue
		}
		return
	}
}
