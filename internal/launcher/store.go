package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ProfileRequest struct {
	ID                   string            `json:"id"`
	Version              string            `json:"version"`
	Ports                []PortMapping     `json:"ports"`
	Env                  map[string]string `json:"env"`
	Resources            Resources         `json:"resources"`
	Enabled              bool              `json:"enabled"`
	Running              bool              `json:"-"`
	RuntimeStatus        string            `json:"runtimeStatus,omitempty"`
	StartingUntil        string            `json:"startingUntil,omitempty"`
	LastAction           string            `json:"lastAction,omitempty"`
	LastActionStatus     string            `json:"lastActionStatus,omitempty"`
	LastActionResult     string            `json:"lastActionResult,omitempty"`
	LastActionAt         string            `json:"lastActionAt,omitempty"`
	LastRequestedVersion string            `json:"lastRequestedVersion,omitempty"`
	ActionLog            []string          `json:"actionLog,omitempty"`
}

type PortMapping struct {
	Container int `json:"container"`
	Host      int `json:"host"`
}

type Resources struct {
	Limits struct {
		Memory string  `json:"memory"`
		CPUs   float64 `json:"cpus"`
	} `json:"limits"`
}

type ProfileStore struct {
	Profiles []ProfileRequest `json:"profiles"`
}

var ErrProfileLimitReached = errors.New("profile limit reached")
var ErrProfileExists = errors.New("profile already exists")

type ValidationError struct {
	Msg string
}

func (e ValidationError) Error() string { return e.Msg }

func (s *Server) createProfile(req ProfileRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := strings.TrimSpace(s.dbPath)
	if path == "" {
		path = filepath.Join(appCfg.DataDir, "profiles.json")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	store, err := loadProfileStore(path)
	if err != nil {
		return err
	}

	for i := range store.Profiles {
		if store.Profiles[i].ID == req.ID {
			return ErrProfileExists
		}
	}
	if len(store.Profiles) >= appCfg.MaxProfiles {
		return ErrProfileLimitReached
	}
	if err := validateCreateConstraints(req, store); err != nil {
		return err
	}

	publicEnv, secretEnv := splitSecretEnv(req.Env)
	if strings.TrimSpace(secretEnv["JWT_SECRET"]) == "" {
		secretEnv["JWT_SECRET"] = randomToken(48)
	}
	if strings.TrimSpace(secretEnv["FLUMIO_ENC_KEY_V0"]) == "" {
		secretEnv["FLUMIO_ENC_KEY_V0"] = randomToken(32)
	}
	req.Env = publicEnv
	req.Enabled = false
	req.Running = false
	req.RuntimeStatus = "stopped"
	req.StartingUntil = ""
	req.LastAction = "create"
	req.LastActionStatus = "success"
	req.LastActionResult = "Profile created"
	req.LastActionAt = time.Now().UTC().Format(time.RFC3339)
	req.ActionLog = []string{req.LastActionAt + " profile created"}
	store.Profiles = append(store.Profiles, req)

	if err := writeProfileStoreAtomic(path, store); err != nil {
		return err
	}
	if err := saveProfileSecrets(req.ID, secretEnv); err != nil {
		return err
	}

	return nil
}

func (s *Server) restoreVersion(id, version string, rollbackOK bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := loadProfileStore(s.dbPath)
	if err != nil {
		return err
	}
	idx := findProfileIndex(store, id)
	if idx < 0 {
		return os.ErrNotExist
	}
	store.Profiles[idx].Version = version
	if rollbackOK {
		store.Profiles[idx].LastActionResult = "Version update failed and rolled back"
	} else {
		store.Profiles[idx].LastActionResult = "Version update failed; rollback also failed"
	}
	store.Profiles[idx].LastAction = "version"
	store.Profiles[idx].LastActionStatus = "failed"
	store.Profiles[idx].LastActionAt = time.Now().UTC().Format(time.RFC3339)
	return writeProfileStoreAtomic(s.dbPath, store)
}

func (s *Server) getProfileForAction(id string) (ProfileStore, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	store, err := loadProfileStore(s.dbPath)
	if err != nil {
		return ProfileStore{}, -1, err
	}
	idx := findProfileIndex(store, id)
	if idx < 0 {
		return ProfileStore{}, -1, os.ErrNotExist
	}
	return store, idx, nil
}

func (s *Server) markProfileResult(id, action, result, message, startingUntil string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := loadProfileStore(s.dbPath)
	if err != nil {
		return err
	}
	idx := findProfileIndex(store, id)
	if idx < 0 {
		return os.ErrNotExist
	}

	now := time.Now().UTC().Format(time.RFC3339)
	profile := &store.Profiles[idx]
	profile.LastAction = action
	profile.LastActionStatus = result
	profile.LastActionAt = now
	profile.LastActionResult = message
	if (action == "enable" || action == "recreate") && result != "failed" {
		profile.Enabled = true
		profile.StartingUntil = startingUntil
	}
	if action == "stop" && result != "failed" {
		profile.Enabled = false
		profile.StartingUntil = ""
	}
	entry := now + " [" + action + "] " + result + ": " + message
	profile.ActionLog = append([]string{entry}, profile.ActionLog...)
	if len(profile.ActionLog) > 8 {
		profile.ActionLog = profile.ActionLog[:8]
	}
	return writeProfileStoreAtomic(s.dbPath, store)
}

func findProfileIndex(store ProfileStore, id string) int {
	for i := range store.Profiles {
		if store.Profiles[i].ID == id {
			return i
		}
	}
	return -1
}

func loadProfileStore(path string) (ProfileStore, error) {
	var store ProfileStore

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProfileStore{Profiles: []ProfileRequest{}}, nil
		}
		return store, err
	}
	if len(bytesTrimSpace(b)) == 0 {
		return ProfileStore{Profiles: []ProfileRequest{}}, nil
	}

	decErr := json.Unmarshal(b, &store)
	if decErr != nil {
		return store, fmt.Errorf("profiles.json is corrupted: %w", decErr)
	}

	if store.Profiles == nil {
		store.Profiles = []ProfileRequest{}
	}

	return store, nil
}

func writeProfileStoreAtomic(path string, store ProfileStore) error {
	tmp := path + ".tmp"

	b, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
