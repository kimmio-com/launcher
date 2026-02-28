package launcher

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"launcher/internal/config"
)

func TestRunCLI_NotHandledForNonProfileCommand(t *testing.T) {
	cfg := config.Load("dev")
	handled, exitCode := RunCLI(cfg, []string{"serve"}, nil, nil)
	if handled {
		t.Fatalf("expected non-profile command to be unhandled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d", exitCode)
	}
}

func TestRunCLI_HandlesLeadingDoubleDash(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Load("dev")
	cfg.DataDir = tmp
	appCfg = cfg

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, exitCode := RunCLI(cfg, []string{"--", "profile", "list"}, &out, &errOut)
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d, err=%s", exitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "No profiles found.") {
		t.Fatalf("expected empty list message, got: %s", out.String())
	}
}

func TestRunCLI_ProfileList(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Load("dev")
	cfg.DataDir = tmp
	appCfg = cfg

	storePath := filepath.Join(cfg.DataDir, "profiles.json")
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{
				ID:      "alpha",
				Version: "1.0.0",
				Ports:   []PortMapping{{Container: 3000, Host: 8088}},
				Env:     map[string]string{"APP_DOMAIN": "localhost"},
				Enabled: false,
			},
		},
	}
	if err := writeProfileStoreAtomic(storePath, store); err != nil {
		t.Fatalf("writeProfileStoreAtomic failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, exitCode := RunCLI(cfg, []string{"profile", "list"}, &out, &errOut)
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d, err=%s", exitCode, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, "alpha") {
		t.Fatalf("expected list output to contain profile id, got: %s", text)
	}
	if !strings.Contains(text, "1.0.0") {
		t.Fatalf("expected list output to contain profile version, got: %s", text)
	}
}

func TestRunCLI_ProfileInfo(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Load("dev")
	cfg.DataDir = tmp
	appCfg = cfg

	storePath := filepath.Join(cfg.DataDir, "profiles.json")
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{
				ID:      "alpha",
				Version: "1.0.0",
				Ports:   []PortMapping{{Container: 3000, Host: 8088}},
				Env:     map[string]string{"APP_DOMAIN": "local.test"},
				Enabled: false,
			},
		},
	}
	if err := writeProfileStoreAtomic(storePath, store); err != nil {
		t.Fatalf("writeProfileStoreAtomic failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, exitCode := RunCLI(cfg, []string{"profile", "alpha", "info"}, &out, &errOut)
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d, err=%s", exitCode, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, "ID: alpha") {
		t.Fatalf("expected info output to contain profile id, got: %s", text)
	}
	if !strings.Contains(text, "Domain: local.test") {
		t.Fatalf("expected info output to contain profile domain, got: %s", text)
	}
}

func TestRunCLI_ProfileUpdateDefaultsToLatest(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Load("dev")
	cfg.DataDir = tmp
	appCfg = cfg

	storePath := filepath.Join(cfg.DataDir, "profiles.json")
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{
				ID:      "alpha",
				Version: "1.0.0",
				Ports:   []PortMapping{{Container: 3000, Host: 8088}},
				Env:     map[string]string{"APP_DOMAIN": "localhost"},
				Enabled: false,
			},
		},
	}
	if err := writeProfileStoreAtomic(storePath, store); err != nil {
		t.Fatalf("writeProfileStoreAtomic failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, exitCode := RunCLI(cfg, []string{"profile", "alpha", "update"}, &out, &errOut)
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d, err=%s", exitCode, errOut.String())
	}

	updated, err := loadProfileStore(storePath)
	if err != nil {
		t.Fatalf("loadProfileStore failed: %v", err)
	}
	if len(updated.Profiles) != 1 {
		t.Fatalf("expected 1 profile after update, got %d", len(updated.Profiles))
	}
	if updated.Profiles[0].Version != "latest" {
		t.Fatalf("expected version latest after update, got %s", updated.Profiles[0].Version)
	}
}

func TestRunCLI_ProfileDelete(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Load("dev")
	cfg.DataDir = tmp
	appCfg = cfg

	storePath := filepath.Join(cfg.DataDir, "profiles.json")
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{
				ID:      "alpha",
				Version: "1.0.0",
				Ports:   []PortMapping{{Container: 3000, Host: 8088}},
				Env:     map[string]string{"APP_DOMAIN": "localhost"},
				Enabled: false,
			},
		},
	}
	if err := writeProfileStoreAtomic(storePath, store); err != nil {
		t.Fatalf("writeProfileStoreAtomic failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, exitCode := RunCLI(cfg, []string{"profile", "alpha", "delete"}, &out, &errOut)
	if !handled {
		t.Fatalf("expected command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d, err=%s", exitCode, errOut.String())
	}

	updated, err := loadProfileStore(storePath)
	if err != nil {
		t.Fatalf("loadProfileStore failed: %v", err)
	}
	if len(updated.Profiles) != 0 {
		t.Fatalf("expected 0 profiles after delete, got %d", len(updated.Profiles))
	}
}
