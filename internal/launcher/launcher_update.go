package launcher

import (
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const launcherRepoLatestReleaseAPI = "https://api.github.com/repos/kimmio-com/launcher/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (s *Server) handleLauncherUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	current := strings.TrimSpace(launcherAppVersion)
	release, err := fetchLatestLauncherRelease()
	if err != nil {
		logWarn("launcher_update_check_failed", map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"currentVersion":  current,
			"latestVersion":   "",
			"updateAvailable": false,
		})
		return
	}

	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	updateAvailable := isNewerVersion(latest, current)
	downloadURL := chooseLauncherAssetURL(release, runtime.GOOS, runtime.GOARCH)
	logInfo("launcher_update_checked", map[string]any{
		"current_version":  current,
		"latest_version":   latest,
		"update_available": updateAvailable,
		"release_url":      release.HTMLURL,
		"download_url_set": downloadURL != "",
		"runtime_goos":     runtime.GOOS,
		"runtime_goarch":   runtime.GOARCH,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"currentVersion":  current,
		"latestVersion":   latest,
		"updateAvailable": updateAvailable,
		"releaseURL":      release.HTMLURL,
		"downloadURL":     downloadURL,
	})
}

func fetchLatestLauncherRelease() (githubRelease, error) {
	var out githubRelease
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, launcherRepoLatestReleaseAPI, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kimmio-launcher")
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return out, errors.New("github release api request failed")
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func chooseLauncherAssetURL(release githubRelease, goos, goarch string) string {
	if len(release.Assets) == 0 {
		return ""
	}
	for _, asset := range release.Assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if name == "" || asset.BrowserDownloadURL == "" {
			continue
		}
		switch goos {
		case "windows":
			if strings.Contains(name, "setup") && strings.HasSuffix(name, ".exe") {
				return asset.BrowserDownloadURL
			}
			if strings.HasSuffix(name, "windows-amd64.zip") {
				return asset.BrowserDownloadURL
			}
		case "darwin":
			if strings.Contains(name, "macos-"+goarch+".dmg") {
				return asset.BrowserDownloadURL
			}
		case "linux":
			if goarch == "amd64" && strings.HasSuffix(name, ".deb") {
				return asset.BrowserDownloadURL
			}
			if strings.HasSuffix(name, ".appimage") {
				return asset.BrowserDownloadURL
			}
			if strings.Contains(name, "linux-amd64.tar.gz") {
				return asset.BrowserDownloadURL
			}
		}
	}
	return ""
}

func isNewerVersion(latest, current string) bool {
	l := parseVersionParts(latest)
	c := parseVersionParts(current)
	max := len(l)
	if len(c) > max {
		max = len(c)
	}
	for i := 0; i < max; i++ {
		lv, cv := 0, 0
		if i < len(l) {
			lv = l[i]
		}
		if i < len(c) {
			cv = c[i]
		}
		if lv > cv {
			return true
		}
		if lv < cv {
			return false
		}
	}
	return false
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if v == "" || v == "dev" {
		return []int{0}
	}
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return []int{0}
	}
	return out
}
