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

	findAsset := func(match func(name string) bool) string {
		for _, asset := range release.Assets {
			name := strings.ToLower(strings.TrimSpace(asset.Name))
			if name == "" || asset.BrowserDownloadURL == "" {
				continue
			}
			if match(name) {
				return asset.BrowserDownloadURL
			}
		}
		return ""
	}

	switch goos {
	case "windows":
		if url := findAsset(func(name string) bool {
			return strings.Contains(name, "setup") && strings.HasSuffix(name, ".exe")
		}); url != "" {
			return url
		}
		if url := findAsset(func(name string) bool { return strings.HasSuffix(name, "windows-amd64.zip") }); url != "" {
			return url
		}
	case "darwin":
		if url := findAsset(func(name string) bool { return strings.Contains(name, "macos-"+goarch+".dmg") }); url != "" {
			return url
		}
	case "linux":
		archToken := "linux-" + strings.ToLower(goarch)
		// Enforce packaging preference regardless of release asset order.
		if url := findAsset(func(name string) bool {
			return strings.HasSuffix(name, ".deb") && strings.Contains(name, archToken)
		}); url != "" {
			return url
		}
		if url := findAsset(func(name string) bool {
			return strings.HasSuffix(name, ".appimage") && strings.Contains(name, archToken)
		}); url != "" {
			return url
		}
		if url := findAsset(func(name string) bool {
			return strings.Contains(name, archToken+".tar.gz")
		}); url != "" {
			return url
		}
	}

	// Last-resort Linux fallback if exact arch artifact is missing.
	if goos == "linux" {
		if url := findAsset(func(name string) bool { return strings.HasSuffix(name, ".deb") }); url != "" {
			return url
		}
		if url := findAsset(func(name string) bool { return strings.HasSuffix(name, ".appimage") }); url != "" {
			return url
		}
		if url := findAsset(func(name string) bool { return strings.Contains(name, "linux-") && strings.HasSuffix(name, ".tar.gz") }); url != "" {
			return url
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
