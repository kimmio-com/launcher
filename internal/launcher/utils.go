package launcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	dockerPathOnce sync.Once
	dockerPath     string
	dockerPathErr  error
)

func dockerBinaryPath() (string, error) {
	dockerPathOnce.Do(func() {
		if p, err := exec.LookPath("docker"); err == nil {
			dockerPath = p
			return
		}

		candidates := []string{
			"/usr/local/bin/docker",
			"/opt/homebrew/bin/docker",
			"/Applications/Docker.app/Contents/Resources/bin/docker",
			"/usr/bin/docker",
			"/snap/bin/docker",
			`C:\Program Files\Docker\Docker\resources\bin\docker.exe`,
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				dockerPath = candidate
				return
			}
		}
		dockerPathErr = errors.New("docker binary not found")
	})
	if dockerPath == "" {
		return "", dockerPathErr
	}
	return dockerPath, nil
}

func IsDockerRunning() string {
	dockerBin, err := dockerBinaryPath()
	if err != nil {
		return "not-installed"
	}

	cmd := dockerCommand(dockerBin, "info")
	if err := cmd.Run(); err != nil {
		return "disabled"
	}

	return "installed"
}

func liveReloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	for {
		fmt.Fprintf(w, "event: ping\ndata: %d\n\n", time.Now().Unix())
		flusher.Flush()
		time.Sleep(1 * time.Second)
	}
}

func openBrowser(port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
	type openTry struct {
		name string
		args []string
	}
	var tries []openTry

	switch runtime.GOOS {
	case "windows":
		tries = []openTry{
			// start requires empty title arg before URL.
			{name: `C:\Windows\System32\cmd.exe`, args: []string{"/c", "start", "", url}},
			{name: "cmd", args: []string{"/c", "start", "", url}},
			{name: "powershell", args: []string{"-NoProfile", "-Command", "Start-Process", url}},
			{name: "rundll32", args: []string{"url.dll,FileProtocolHandler", url}},
		}
	case "darwin":
		tries = []openTry{
			{name: "open", args: []string{url}},
		}
	default:
		tries = []openTry{
			{name: "xdg-open", args: []string{url}},
		}
	}

	var failures []string
	for _, t := range tries {
		cmd := exec.Command(t.name, t.args...)
		if err := cmd.Start(); err == nil {
			logInfo("browser_opened", map[string]any{"url": url, "method": t.name})
			return
		} else {
			failures = append(failures, t.name+": "+err.Error())
		}
	}
	logWarn("browser_open_failed", map[string]any{
		"url":    url,
		"errors": strings.Join(failures, " | "),
	})
}

func dockerCommand(dockerBin string, args ...string) *exec.Cmd {
	cmd := exec.Command(dockerBin, args...)
	cmd.Env = dockerCommandEnv()
	return cmd
}

func dockerCommandWithContext(ctx context.Context, dockerBin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, dockerBin, args...)
	cmd.Env = dockerCommandEnv()
	return cmd
}

func dockerCommandEnv() []string {
	env := os.Environ()
	if strings.TrimSpace(os.Getenv("DOCKER_HOST")) != "" {
		return env
	}
	// Desktop/icon launches may miss shell-initialized DOCKER_HOST for rootless Docker.
	xdgRuntime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if xdgRuntime != "" {
		sock := filepath.Join(xdgRuntime, "docker.sock")
		if info, err := os.Stat(sock); err == nil && !info.IsDir() {
			return append(env, "DOCKER_HOST=unix://"+sock)
		}
	}
	uid := strings.TrimSpace(os.Getenv("UID"))
	if uid != "" {
		sock := filepath.Join("/run/user", uid, "docker.sock")
		if info, err := os.Stat(sock); err == nil && !info.IsDir() {
			return append(env, "DOCKER_HOST=unix://"+sock)
		}
	}
	return env
}
