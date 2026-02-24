package launcher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"launcher/internal/config"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitSecretEnv(t *testing.T) {
	in := map[string]string{
		"JWT_SECRET": "jwt",
		"ENC_KEY_V0": "enc",
		"APP_DOMAIN": "localhost",
	}
	publicEnv, secretEnv := splitSecretEnv(in)

	if _, ok := publicEnv["JWT_SECRET"]; ok {
		t.Fatalf("JWT_SECRET should not be in public env")
	}
	if _, ok := publicEnv["ENC_KEY_V0"]; ok {
		t.Fatalf("ENC_KEY_V0 should not be in public env")
	}
	if publicEnv["APP_DOMAIN"] != "localhost" {
		t.Fatalf("APP_DOMAIN should stay public")
	}
	if secretEnv["JWT_SECRET"] != "jwt" || secretEnv["ENC_KEY_V0"] != "enc" {
		t.Fatalf("secret env values mismatch")
	}
}

func TestCreateProfileStoresSecretsOutsideProfilesJSON(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	cfg := config.Load("dev")
	appCfg = cfg
	srv := NewServer(cfg)
	srv.dbPath = filepath.Join(tmp, "profiles.json")

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to pick free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	req := ProfileRequest{
		ID:      "kimmio-default",
		Version: "latest",
		Ports:   []PortMapping{{Container: 3000, Host: port}},
		Env: map[string]string{
			"APP_DOMAIN": "localhost",
			"JWT_SECRET": "jwt-secret-test",
			"ENC_KEY_V0": "enc-secret-test",
		},
	}

	if err := srv.createProfile(req); err != nil {
		t.Fatalf("createProfile failed: %v", err)
	}

	store, err := loadProfileStore(srv.dbPath)
	if err != nil {
		t.Fatalf("loadProfileStore failed: %v", err)
	}
	if len(store.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(store.Profiles))
	}
	if _, ok := store.Profiles[0].Env["JWT_SECRET"]; ok {
		t.Fatalf("JWT_SECRET should not be persisted in profiles.json")
	}
	if _, ok := store.Profiles[0].Env["ENC_KEY_V0"]; ok {
		t.Fatalf("ENC_KEY_V0 should not be persisted in profiles.json")
	}

	loadedSecrets := loadProfileSecrets(req.ID)
	if loadedSecrets["JWT_SECRET"] != "jwt-secret-test" {
		t.Fatalf("JWT secret not stored in secrets file")
	}
	if loadedSecrets["ENC_KEY_V0"] != "enc-secret-test" {
		t.Fatalf("enc key not stored in secrets file")
	}
	if store.Profiles[0].Ports[0].Host != port {
		t.Fatalf("expected stored host port %d, got %d", port, store.Profiles[0].Ports[0].Host)
	}
}

func TestCreateProfileGeneratesSecretsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWD) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	cfg := config.Load("dev")
	appCfg = cfg
	srv := NewServer(cfg)
	srv.dbPath = filepath.Join(tmp, "profiles.json")

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to pick free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	req := ProfileRequest{
		ID:      "kimmio-generated-secrets",
		Version: "latest",
		Ports:   []PortMapping{{Container: 3000, Host: port}},
		Env: map[string]string{
			"APP_DOMAIN": "localhost",
		},
	}

	if err := srv.createProfile(req); err != nil {
		t.Fatalf("createProfile failed: %v", err)
	}

	loadedSecrets := loadProfileSecrets(req.ID)
	jwt := loadedSecrets["JWT_SECRET"]
	enc := loadedSecrets["ENC_KEY_V0"]
	if len(jwt) < 32 {
		t.Fatalf("expected generated JWT_SECRET length >= 32, got %d", len(jwt))
	}
	decoded, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("expected generated ENC_KEY_V0 to be base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected generated ENC_KEY_V0 to decode to 32 bytes, got %d", len(decoded))
	}
}

func TestParseVersionFromRequest_JSON(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"version": "1.0.1"})
	r, err := http.NewRequest(http.MethodPost, "/api/profiles/x/version", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")

	got, err := parseVersionFromRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "1.0.1" {
		t.Fatalf("expected 1.0.1, got %q", got)
	}
}

func TestParseVersionFromRequest_Form(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, "/api/profiles/x/version", bytes.NewBufferString("version=latest"))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	got, err := parseVersionFromRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "latest" {
		t.Fatalf("expected latest, got %q", got)
	}
}

func TestParseVersionFromRequest_InvalidTag(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"version": "bad/tag"})
	r, err := http.NewRequest(http.MethodPost, "/api/profiles/x/version", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")

	_, err = parseVersionFromRequest(r)
	if err == nil {
		t.Fatalf("expected invalid version tag error")
	}
}

func TestIsValidDomain(t *testing.T) {
	if !isValidDomain("localhost") {
		t.Fatalf("localhost should be valid")
	}
	if !isValidDomain("app.example.com") {
		t.Fatalf("fqdn should be valid")
	}
	if isValidDomain("http://localhost") {
		t.Fatalf("domain with scheme should be invalid")
	}
	if isValidDomain("bad domain") {
		t.Fatalf("domain with spaces should be invalid")
	}
}

func TestValidateCreateConstraints_DuplicatePort(t *testing.T) {
	req := ProfileRequest{
		ID:    "kimmio-2",
		Ports: []PortMapping{{Container: 3000, Host: 8088}},
	}
	store := ProfileStore{
		Profiles: []ProfileRequest{
			{ID: "kimmio-default", Ports: []PortMapping{{Container: 3000, Host: 8088}}},
		},
	}
	err := validateCreateConstraints(req, store)
	if err == nil {
		t.Fatalf("expected duplicate port validation error")
	}
}
