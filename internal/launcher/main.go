package launcher

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"launcher/internal/config"
)

type Server struct {
	dbPath         string
	mu             sync.Mutex
	jobMu          sync.Mutex
	jobs           map[string]*ActionJob
	activeProfiles map[string]string
	jobCancels     map[string]context.CancelFunc
}

var appCfg = config.Load("dev")
var launcherAppVersion = "dev"
var launcherGitCommit = "unknown"

func SetBuildInfo(version, commit string) {
	v := strings.TrimSpace(version)
	c := strings.TrimSpace(commit)
	if v != "" {
		launcherAppVersion = v
	}
	if c != "" {
		launcherGitCommit = c
	}
}

func NewServer(cfg config.Config) *Server {
	return &Server{
		dbPath:         filepath.Join(cfg.DataDir, "profiles.json"),
		jobs:           map[string]*ActionJob{},
		activeProfiles: map[string]string{},
		jobCancels:     map[string]context.CancelFunc{},
	}
}

func Run(embedded fs.FS, cfg config.Config) error {
	appCfg = cfg
	initStructuredLogger(cfg.DataDir)
	port := resolveListenPort(cfg.ListenPort, cfg.PortSearchRange)
	writeLauncherPortFile(port)

	ts, err := NewTemplatesFromFS(embedded, "templates")
	if err != nil {
		return fmt.Errorf("templates: %w", err)
	}

	srv := NewServer(cfg)

	staticFS, err := fs.Sub(embedded, "static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		csrfToken := ensureCSRFCookie(w, r)
		store := ProfileStore{Profiles: []ProfileRequest{}}
		b, err := os.ReadFile(srv.dbPath)
		if err == nil && len(strings.TrimSpace(string(b))) > 0 {
			_ = json.Unmarshal(b, &store)
		}
		store.Profiles = applyHealthStatus(store.Profiles)
		if err := ts.RenderPageWithTemplate(w, "profiles.html", map[string]any{
			"DockerRunning": IsDockerRunning(),
			"Profiles":      srv.attachActiveJobs(store.Profiles),
			"ProfileCount":  len(store.Profiles),
			"MaxProfiles":   appCfg.MaxProfiles,
			"CSRFToken":     csrfToken,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/profiles/new", func(w http.ResponseWriter, r *http.Request) {
		csrfToken := ensureCSRFCookie(w, r)
		store, err := loadProfileStore(srv.dbPath)
		if err != nil {
			http.Error(w, "Failed to load profiles: "+err.Error(), http.StatusInternalServerError)
			return
		}
		profile := defaultProfile()
		profile.ID = nextAvailableProfileID(store)
		profile.Ports[0].Host = nextAvailablePort(store)
		if err := ts.RenderPageWithTemplate(w, "profile-create.html", map[string]any{
			"DockerRunning": IsDockerRunning(),
			"Profile":       profile,
			"HostPort":      profile.Ports[0].Host,
			"IsEdit":        false,
			"ProfileCount":  len(store.Profiles),
			"MaxProfiles":   appCfg.MaxProfiles,
			"MaxReached":    len(store.Profiles) >= appCfg.MaxProfiles,
			"CSRFToken":     csrfToken,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/profiles/edit", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Profile updates are disabled", http.StatusForbidden)
	})

	mux.HandleFunc("/api/profiles", withMutationGuard(srv.handleCreateProfile))
	mux.HandleFunc("/api/profiles/", withMutationGuard(srv.handleProfileAction))
	mux.HandleFunc("/api/jobs/", withMutationGuard(srv.handleJobRoute))
	mux.HandleFunc("/api/kimmio/versions", srv.handleKimmioVersions)
	mux.HandleFunc("/api/launcher/update", srv.handleLauncherUpdate)
	mux.HandleFunc("/api/server/stop", withMutationGuard(handleServerStop))
	mux.HandleFunc("/__livereload", liveReloadHandler)

	launcherURL := fmt.Sprintf("http://localhost:%d", port)
	printStartupBanner(launcherURL)

	if cfg.BuildMode == "prod" {
		go openBrowserWhenReachable(port, 12*time.Second)
	}
	logInfo("server_start", map[string]any{
		"port":           port,
		"url":            launcherURL,
		"data_dir":       cfg.DataDir,
		"build_mode":     cfg.BuildMode,
		"app_version":    launcherAppVersion,
		"build_commit":   launcherGitCommit,
		"runtime_goos":   runtime.GOOS,
		"runtime_goarch": runtime.GOARCH,
	})
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

func printStartupBanner(url string) {
	if runtime.GOOS == "windows" || strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		fmt.Println("Kimmio Launcher")
		fmt.Println("Welcome to Kimmio Launcher")
		fmt.Printf("To visit it go to URL: %s\n", url)
		fmt.Println(url)
		return
	}

	const (
		reset      = "\033[0m"
		bold       = "\033[1m"
		cyan       = "\033[36m"
		green      = "\033[32m"
		brightGray = "\033[90m"
	)

	fmt.Printf("%s%sKimmio Launcher%s\n", bold, cyan, reset)
	fmt.Printf("%sWelcome to Kimmio Launcher%s\n", green, reset)
	fmt.Printf("%sTo visit it go to URL:%s %s%s%s\n", brightGray, reset, bold, url, reset)
	// Standalone URL line improves click-detection in Linux terminals.
	fmt.Println(url)
	// OSC 8 hyperlink (supported by many modern terminals).
	fmt.Printf("\033]8;;%s\033\\Open Kimmio Launcher\033]8;;\033\\\n", url)
}

func openBrowserWhenReachable(port int, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 300*time.Millisecond); err == nil {
			_ = conn.Close()
			openBrowser(port)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Fallback attempt even if readiness probe timed out.
	openBrowser(port)
}

func handleServerStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Launcher stopping",
	})
	fmt.Println("Stopping server...")
	logInfo("server_stopping", map[string]any{"reason": "api_server_stop"})

	go func() {
		time.Sleep(220 * time.Millisecond)
		os.Exit(0)
	}()
}

func writeLauncherPortFile(currentPort int) {
	if currentPort <= 0 {
		return
	}
	if err := os.MkdirAll(appCfg.DataDir, 0o755); err != nil {
		logError("runtime_data_dir_create_failed", map[string]any{"error": err.Error(), "data_dir": appCfg.DataDir})
		return
	}
	portFile := filepath.Join(appCfg.DataDir, "launcher-port")
	if err := os.WriteFile(portFile, []byte(strconv.Itoa(currentPort)+"\n"), 0o644); err != nil {
		logError("launcher_port_write_failed", map[string]any{"error": err.Error(), "port_file": portFile})
	}
}

func resolveListenPort(preferredPort, searchRange int) int {
	if preferredPort <= 0 {
		preferredPort = 7331
	}
	if isTCPPortAvailable(preferredPort) {
		return preferredPort
	}
	for p := preferredPort + 1; p <= preferredPort+searchRange; p++ {
		if isTCPPortAvailable(p) {
			logWarn("listen_port_fallback", map[string]any{"preferred_port": preferredPort, "selected_port": p})
			return p
		}
	}
	logWarn("listen_port_unavailable_range", map[string]any{"preferred_port": preferredPort, "search_range": searchRange})
	return preferredPort
}

func defaultProfile() ProfileRequest {
	profile := ProfileRequest{
		ID:      "kimmio-default",
		Version: "latest",
		Ports: []PortMapping{
			{Container: 3000, Host: appCfg.ProfilePortMin},
		},
		Env: map[string]string{
			"APP_DOMAIN": "localhost",
			"JWT_SECRET": randomToken(48),
			"ENC_KEY_V0": randomBase64Key32(),
		},
	}
	profile.Resources.Limits.Memory = ""
	profile.Resources.Limits.CPUs = 0
	return profile
}

func nextAvailablePort(store ProfileStore) int {
	used := map[int]bool{}
	for _, profile := range store.Profiles {
		if len(profile.Ports) > 0 && profile.Ports[0].Host > 0 {
			used[profile.Ports[0].Host] = true
		}
	}
	for p := appCfg.ProfilePortMin; p < appCfg.ProfilePortMax; p++ {
		if !used[p] && isTCPPortAvailable(p) {
			return p
		}
	}
	return appCfg.ProfilePortMin
}

func nextAvailableProfileID(store ProfileStore) string {
	used := map[string]bool{}
	for _, profile := range store.Profiles {
		used[profile.ID] = true
	}

	if !used["kimmio-default"] {
		return "kimmio-default"
	}

	for i := 2; i < 1000; i++ {
		candidate := "kimmio-" + strconv.Itoa(i)
		if !used[candidate] {
			return candidate
		}
	}
	return "kimmio-" + strconv.FormatInt(int64(len(store.Profiles)+1), 10)
}

func isTCPPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func randomToken(minLen int) string {
	if minLen < 32 {
		minLen = 32
	}
	buf := make([]byte, minLen)
	if _, err := rand.Read(buf); err != nil {
		return "change-this-secret-please-1234567890"
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	if len(token) >= minLen {
		return token[:minLen]
	}
	return token
}

func randomBase64Key32() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// 32 ASCII bytes -> base64 decodes back to 32 bytes.
		return base64.StdEncoding.EncodeToString([]byte("change-this-enc-key-now-please-32b"))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func normalizeEncryptionKeyValue(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if dec, err := base64.StdEncoding.DecodeString(v); err == nil && len(dec) == 32 {
		return v, true
	}
	if dec, err := base64.RawStdEncoding.DecodeString(v); err == nil && len(dec) == 32 {
		return base64.StdEncoding.EncodeToString(dec), true
	}
	// Backward compatibility: older launcher stored raw 32-char strings.
	if len(v) == 32 {
		return base64.StdEncoding.EncodeToString([]byte(v)), true
	}
	return "", false
}

func applyHealthStatus(profiles []ProfileRequest) []ProfileRequest {
	updated := make([]ProfileRequest, len(profiles))
	copy(updated, profiles)
	for i := range updated {
		profile := &updated[i]
		profile.Running = false
		profile.RuntimeStatus = "stopped"

		if !profile.Enabled {
			continue
		}

		if isWithinStartingWindow(profile.StartingUntil) {
			if retryProfileHealth(*profile, 2, 400*time.Millisecond) {
				profile.Running = true
				profile.RuntimeStatus = "running"
			} else {
				profile.RuntimeStatus = "starting"
			}
			continue
		}

		if retryProfileHealth(*profile, 4, 500*time.Millisecond) {
			profile.Running = true
			profile.RuntimeStatus = "running"
		} else {
			profile.RuntimeStatus = "unhealthy"
		}
	}
	return updated
}

func (s *Server) attachActiveJobs(profiles []ProfileRequest) []ProfileRequest {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	out := make([]ProfileRequest, len(profiles))
	copy(out, profiles)
	for i := range out {
		out[i].ActiveJobID = s.activeProfiles[out[i].ID]
	}
	return out
}

func isWithinStartingWindow(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return false
	}
	return time.Now().UTC().Before(t)
}

func retryProfileHealth(profile ProfileRequest, attempts int, sleep time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if isProfileHealthy(profile) {
			return true
		}
		time.Sleep(sleep)
	}
	return false
}

func isProfileHealthy(profile ProfileRequest) bool {
	hostPort := 0
	if len(profile.Ports) > 0 {
		hostPort = profile.Ports[0].Host
	}
	if hostPort <= 0 {
		return false
	}

	client := http.Client{Timeout: 2 * time.Second}
	url := "http://localhost:" + strconv.Itoa(hostPort) + "/health"
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
