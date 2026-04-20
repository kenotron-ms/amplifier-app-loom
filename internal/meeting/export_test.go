//go:build darwin && cgo

package meeting

import sh "github.com/kenotron-ms/side-huddle/bindings/go"

// NoOpListenerFactory is a test seam that skips real CoreAudio monitoring.
var NoOpListenerFactory = func(_ string) (*sh.Listener, error) {
	return nil, nil
}

// HandleEventForTest exposes handleEvent for white-box testing.
func (s *Service) HandleEventForTest(e *sh.Event) {
	s.handleEvent(e)
}