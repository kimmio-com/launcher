package launcher

import (
	"luncher/internal/config"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestNextAvailableProfileID(t *testing.T) {
	appCfg = config.Load("dev")
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{ID: "kimmio-default"},
			{ID: "kimmio-2"},
		},
	}

	got := nextAvailableProfileID(store)
	if got != "kimmio-3" {
		t.Fatalf("expected kimmio-3, got %q", got)
	}
}

func TestIsWithinStartingWindow(t *testing.T) {
	future := time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339)
	if !isWithinStartingWindow(future) {
		t.Fatalf("expected future timestamp to be within starting window")
	}

	past := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	if isWithinStartingWindow(past) {
		t.Fatalf("expected past timestamp not to be within starting window")
	}
}

func TestApplyHealthStatusStarting(t *testing.T) {
	appCfg = config.Load("dev")
	profiles := []ProfileRequest{
		{
			ID:            "p1",
			Enabled:       true,
			StartingUntil: time.Now().UTC().Add(20 * time.Second).Format(time.RFC3339),
			Ports:         []PortMapping{{Container: 3000, Host: 65534}},
		},
	}

	got := applyHealthStatus(profiles)
	if len(got) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(got))
	}
	if got[0].RuntimeStatus != "starting" {
		t.Fatalf("expected runtimeStatus=starting, got %q", got[0].RuntimeStatus)
	}
	if got[0].Running {
		t.Fatalf("expected running=false while health is not ready")
	}
}

func TestResolveListenPortFallback(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen on random port: %v", err)
	}
	defer ln.Close()

	used := ln.Addr().(*net.TCPAddr).Port
	got := resolveListenPort(used, 10)
	if got == used {
		t.Fatalf("expected fallback port, got busy port %d", got)
	}
	if !isTCPPortAvailable(got) {
		t.Fatalf("expected resolved port %d to be available", got)
	}
}

func TestResolveListenPortInvalidInput(t *testing.T) {
	appCfg = config.Load("dev")
	got := resolveListenPort(0, 0)
	if got != 7331 {
		t.Fatalf("expected default port 7331, got %s", strconv.Itoa(got))
	}
}
