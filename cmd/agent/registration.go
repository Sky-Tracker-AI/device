package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/skytracker/skytracker-device/internal/hwinfo"
	"github.com/skytracker/skytracker-device/internal/platform"
	"github.com/skytracker/skytracker-device/internal/state"
)

// registrationHelper encapsulates retry logic for device registration.
// It is intended to be called from runPlatformSync on each health tick
// when the device is not yet registered.
type registrationHelper struct {
	retrier    *platform.RegistrationRetrier
	platHolder *platformClientHolder
	agentState *state.State
	endpoint   string
	hwStatic   hwinfo.StaticInfo
}

func newRegistrationHelper(platHolder *platformClientHolder, agentState *state.State, endpoint string, hwStatic hwinfo.StaticInfo) *registrationHelper {
	h := &registrationHelper{
		retrier:    platform.NewRegistrationRetrier(),
		platHolder: platHolder,
		agentState: agentState,
		endpoint:   endpoint,
		hwStatic:   hwStatic,
	}
	if agentState.IsRegistered() {
		h.retrier.MarkRegistered()
	}
	return h
}

// tryRegister attempts registration if not already registered, using
// exponential backoff. Returns true if the device is registered (either
// already was, or just succeeded).
func (h *registrationHelper) tryRegister(ctx context.Context, gps gpsInterface) bool {
	if h.agentState.IsRegistered() {
		return true
	}

	resp := h.retrier.AttemptRegistration(ctx, func(ctx context.Context) (*platform.RegisterResponse, error) {
		pos := gps.Position()
		client := h.platHolder.Get()
		if client == nil {
			return nil, fmt.Errorf("no platform client")
		}
		return client.Register(ctx, platform.RegisterRequest{
			Serial:        h.agentState.GetSerial(),
			HardwareInfo:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			OSVersion:     h.hwStatic.OSPrettyName,
			AgentVersion:  version,
			Lat:           pos.Lat,
			Lon:           pos.Lon,
			BoardModel:    h.hwStatic.BoardModel,
			CPUModel:      h.hwStatic.CPUModel,
			KernelVersion: h.hwStatic.KernelVersion,
			TotalMemoryMB: h.hwStatic.TotalMemoryMB,
		})
	})

	if resp == nil {
		return false
	}

	h.agentState.SetRegistration(resp.DeviceID, resp.APIKey, resp.StationID, resp.ClaimCode)
	if err := h.agentState.Save(); err != nil {
		log.Printf("[platform] failed to save state after registration: %v", err)
	}
	// Re-create client with the new API key.
	h.platHolder.Set(platform.NewClient(h.endpoint, h.agentState.GetAPIKey()))
	log.Printf("[platform] registered (retry): device=%s station=%s claim_code=%s",
		h.agentState.GetDeviceID(), h.agentState.GetStationID(), h.agentState.GetClaimCode())
	return true
}
