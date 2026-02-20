package launcher

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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

	cmd := exec.Command(dockerBin, "info")
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
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}
