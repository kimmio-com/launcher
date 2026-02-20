package launcher

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *Server) handleKimmioVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	versions := fetchKnownKimmioVersions()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"versions": versions,
	})
}

func fetchKnownKimmioVersions() []string {
	fallback := []string{"latest", "1.0.1", "1.0.0"}

	client := http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "https://registry.hub.docker.com/v2/repositories/kimmio/kimmio-app/tags?page_size=20", nil)
	resp, err := client.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fallback
	}

	var payload struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fallback
	}

	set := map[string]bool{"latest": true}
	for _, r := range payload.Results {
		tag := strings.TrimSpace(r.Name)
		if tag == "" {
			continue
		}
		if versionTagRe.MatchString(tag) {
			set[tag] = true
		}
	}

	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == "latest" {
			return true
		}
		if out[j] == "latest" {
			return false
		}
		return out[i] > out[j]
	})
	return out
}
