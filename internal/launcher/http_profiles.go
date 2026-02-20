package launcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var profileIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)
var versionTagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var domainRe = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, fromForm, err := decodeProfileRequest(r)
	if err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateAndNormalize(&req); err != nil {
		http.Error(w, "Validation error: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = s.createProfile(req)
	if err != nil {
		if errors.Is(err, ErrProfileLimitReached) {
			http.Error(w, fmt.Sprintf("Validation error: profile limit reached (max %d)", appCfg.MaxProfiles), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrProfileExists) {
			http.Error(w, "Validation error: "+err.Error(), http.StatusBadRequest)
			return
		}
		var ve ValidationError
		if errors.As(err, &ve) {
			http.Error(w, "Validation error: "+ve.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if fromForm {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":      true,
		"created": true,
		"profile": req,
	})
}

func decodeProfileRequest(r *http.Request) (ProfileRequest, bool, error) {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))

	if ct == "application/json" || ct == "" {
		var req ProfileRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err == nil {
			return req, false, nil
		}
		if ct == "application/json" {
			return ProfileRequest{}, false, errors.New("invalid JSON body")
		}
	}

	if err := r.ParseForm(); err != nil {
		return ProfileRequest{}, true, fmt.Errorf("failed to parse form: %w", err)
	}

	id := strings.TrimSpace(r.FormValue("id"))
	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		version = strings.TrimSpace(r.FormValue("version"))
	}

	hostPortStr := strings.TrimSpace(r.FormValue("hostPort"))
	hostPort, _ := strconv.Atoi(hostPortStr)
	domain := strings.TrimSpace(r.FormValue("domain"))

	mem := strings.TrimSpace(r.FormValue("memory"))
	jwtSecret := strings.TrimSpace(r.FormValue("jwtSecret"))
	flumioEncKeyV0 := strings.TrimSpace(r.FormValue("flumioEncKeyV0"))

	cpusStr := strings.TrimSpace(r.FormValue("cpus"))
	var cpus float64
	if cpusStr != "" {
		cpus, _ = strconv.ParseFloat(cpusStr, 64)
	}

	req := ProfileRequest{
		ID:      id,
		Version: version,
		Ports: []PortMapping{
			{Container: 3000, Host: hostPort},
		},
		Env: map[string]string{},
	}
	if jwtSecret != "" {
		req.Env["JWT_SECRET"] = jwtSecret
	}
	if flumioEncKeyV0 != "" {
		req.Env["FLUMIO_ENC_KEY_V0"] = flumioEncKeyV0
	}
	if domain != "" {
		req.Env["APP_DOMAIN"] = domain
	}
	req.Resources.Limits.Memory = mem
	req.Resources.Limits.CPUs = cpus

	return req, true, nil
}

func validateAndNormalize(req *ProfileRequest) error {
	req.ID = strings.ToLower(strings.TrimSpace(req.ID))
	req.Version = strings.TrimSpace(req.Version)

	if !profileIDRe.MatchString(req.ID) {
		return errors.New("id must be lowercase letters/numbers/dashes, length 3-64 (e.g. omega-production-01)")
	}

	if req.Version == "" {
		req.Version = "latest"
	}

	if len(req.Ports) == 0 {
		req.Ports = []PortMapping{{Container: 3000, Host: 8080}}
	}
	if req.Ports[0].Host <= 0 || req.Ports[0].Host > 65535 {
		return errors.New("host port must be in range 1..65535")
	}
	if req.Ports[0].Container <= 0 || req.Ports[0].Container > 65535 {
		req.Ports[0].Container = 3000
	}

	mem := strings.TrimSpace(req.Resources.Limits.Memory)
	if mem != "" && !isValidMem(mem) {
		return errors.New("memory must look like 512mb / 1gb / 2g / 4096m (or empty for default)")
	}
	req.Resources.Limits.Memory = mem

	if req.Resources.Limits.CPUs < 0 {
		return errors.New("cpus cannot be negative")
	}

	if req.Env == nil {
		req.Env = map[string]string{}
	}
	for k := range req.Env {
		if !isSafeEnvKey(k) {
			return fmt.Errorf("invalid env key: %q", k)
		}
	}
	if domain := strings.TrimSpace(req.Env["APP_DOMAIN"]); domain != "" && !isValidDomain(domain) {
		return errors.New("domain must be hostname only (example: localhost or app.example.com)")
	}
	if key := strings.TrimSpace(req.Env["FLUMIO_ENC_KEY_V0"]); key != "" && len(key) != 32 {
		return errors.New("FLUMIO_ENC_KEY_V0 must be exactly 32 characters")
	}
	if jwt := strings.TrimSpace(req.Env["JWT_SECRET"]); jwt != "" && len(jwt) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}

	return nil
}

func isValidMem(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	memRe := regexp.MustCompile(`^\d+(\.\d+)?\s*(b|k|kb|m|mb|g|gb)$`)
	return memRe.MatchString(strings.ReplaceAll(v, " ", ""))
}

func isSafeEnvKey(k string) bool {
	keyRe := regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
	return keyRe.MatchString(k)
}

func (s *Server) handleProfileAction(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(trimmed, "/")
	id := strings.ToLower(strings.TrimSpace(parts[0]))
	if !profileIDRe.MatchString(id) {
		http.Error(w, "Invalid profile id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		job, err := s.enqueueProfileJob(id, "delete", func(jobID string) error {
			s.updateJobStep(jobID, "down", "running", "Stopping profile", 20, "")
			return s.performDelete(id, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	}

	if len(parts) != 2 || r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := strings.ToLower(strings.TrimSpace(parts[1]))
	switch action {
	case "enable":
		job, err := s.enqueueProfileJob(id, action, func(jobID string) error {
			return s.performEnable(id, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	case "stop":
		job, err := s.enqueueProfileJob(id, action, func(jobID string) error {
			return s.performStop(id, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	case "recreate":
		job, err := s.enqueueProfileJob(id, action, func(jobID string) error {
			return s.performRecreate(id, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	case "version":
		newVersion, err := parseVersionFromRequest(r)
		if err != nil {
			http.Error(w, "Version update failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		job, err := s.enqueueProfileJob(id, action, func(jobID string) error {
			return s.performVersionUpdate(id, newVersion, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	case "regenerate-secrets":
		job, err := s.enqueueProfileJob(id, action, func(jobID string) error {
			return s.performRegenerateSecrets(id, jobID)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "jobId": job.ID})
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func parseVersionFromRequest(r *http.Request) (string, error) {
	newVersion := strings.TrimSpace(r.FormValue("version"))
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		var body struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", errors.New("invalid JSON body")
		}
		newVersion = strings.TrimSpace(body.Version)
	}
	if newVersion == "" {
		return "", errors.New("version is required")
	}
	if !versionTagRe.MatchString(newVersion) {
		return "", errors.New("invalid version tag")
	}
	return newVersion, nil
}

func validateCreateConstraints(req ProfileRequest, store ProfileStore) error {
	if len(req.Ports) == 0 {
		return ValidationError{Msg: "host port is required"}
	}
	hostPort := req.Ports[0].Host
	if hostPort < 1024 {
		return ValidationError{Msg: "host port must be >= 1024 (reserved ports are blocked)"}
	}
	reserved := map[int]bool{appCfg.ListenPort: true}
	if reserved[hostPort] {
		return ValidationError{Msg: fmt.Sprintf("host port %d is reserved", hostPort)}
	}
	for _, p := range store.Profiles {
		if len(p.Ports) > 0 && p.Ports[0].Host == hostPort {
			return ValidationError{Msg: fmt.Sprintf("host port %d is already used by profile %s", hostPort, p.ID)}
		}
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(hostPort))
	if err != nil {
		return ValidationError{Msg: fmt.Sprintf("host port %d is unavailable on this machine", hostPort)}
	}
	_ = ln.Close()
	return nil
}

func isValidDomain(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || len(v) > 253 {
		return false
	}
	if strings.Contains(v, "://") || strings.Contains(v, "/") || strings.Contains(v, " ") {
		return false
	}
	if !domainRe.MatchString(v) {
		return false
	}
	parts := strings.Split(v, ".")
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return false
		}
	}
	return true
}
