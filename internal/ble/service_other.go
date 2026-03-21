//go:build !linux

package ble

import (
	"context"
	"log"
)

// serviceImpl is a no-op stub on non-Linux platforms.
type serviceImpl struct{}

func newServiceImpl(s *Service) serviceImpl {
	return serviceImpl{}
}

func (si *serviceImpl) setRegisterFunc(fn RegisterFunc) {}

func (si *serviceImpl) run(ctx context.Context) {
	log.Printf("[ble] BLE provisioning not supported on this platform")
	<-ctx.Done()
}

func (si *serviceImpl) startAdvertising() {}

func (si *serviceImpl) onClaimed() {}
