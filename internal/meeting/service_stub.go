//go:build !darwin || !cgo

package meeting

import "context"

// Service is a no-op stub on non-darwin/non-cgo platforms.
type Service struct{}

func NewService(_ *ConfigStore, _ Notifier, _ *Transcriber) *Service { return &Service{} }
func (s *Service) Start(_ context.Context) error                     { return nil }
func (s *Service) Stop()                                             {}
func (s *Service) SetEnabled(_ context.Context, _ bool) error        { return nil }
func (s *Service) State() State                                      { return StateIdle }
