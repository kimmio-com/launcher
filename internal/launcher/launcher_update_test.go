package launcher

import "testing"

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "1.0.1", current: "1.0.0", want: true},
		{latest: "1.1.0", current: "1.0.9", want: true},
		{latest: "1.0.0", current: "1.0.0", want: false},
		{latest: "1.0.0", current: "dev", want: true},
		{latest: "1.0.0", current: "1.2.0", want: false},
		{latest: "v1.2.0", current: "1.1.9", want: true},
		{latest: "1.0.0-rc1", current: "0.9.9", want: true},
	}
	for _, tc := range tests {
		got := isNewerVersion(tc.latest, tc.current)
		if got != tc.want {
			t.Fatalf("isNewerVersion(%q,%q)=%v want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestChooseLauncherAssetURL(t *testing.T) {
	release := githubRelease{
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{
			{Name: "Kimmio-Launcher-Setup-windows-amd64.exe", BrowserDownloadURL: "https://example/setup.exe"},
			{Name: "Kimmio-Launcher-1.2.0-macos-arm64.dmg", BrowserDownloadURL: "https://example/macos-arm64.dmg"},
			{Name: "Kimmio-Launcher-1.2.0-macos-amd64.dmg", BrowserDownloadURL: "https://example/macos-amd64.dmg"},
			{Name: "Kimmio-Launcher-1.2.0-linux-amd64.deb", BrowserDownloadURL: "https://example/linux.deb"},
			{Name: "Kimmio-Launcher-1.2.0-linux-arm64.deb", BrowserDownloadURL: "https://example/linux-arm64.deb"},
		},
	}

	if got := chooseLauncherAssetURL(release, "windows", "amd64"); got != "https://example/setup.exe" {
		t.Fatalf("windows asset mismatch: %s", got)
	}
	if got := chooseLauncherAssetURL(release, "darwin", "arm64"); got != "https://example/macos-arm64.dmg" {
		t.Fatalf("darwin arm64 asset mismatch: %s", got)
	}
	if got := chooseLauncherAssetURL(release, "darwin", "amd64"); got != "https://example/macos-amd64.dmg" {
		t.Fatalf("darwin amd64 asset mismatch: %s", got)
	}
	if got := chooseLauncherAssetURL(release, "linux", "amd64"); got != "https://example/linux.deb" {
		t.Fatalf("linux asset mismatch: %s", got)
	}
	if got := chooseLauncherAssetURL(release, "linux", "arm64"); got != "https://example/linux-arm64.deb" {
		t.Fatalf("linux arm64 asset mismatch: %s", got)
	}
}

func TestChooseLauncherAssetURLLinuxPrefersDebOverArchiveOrder(t *testing.T) {
	release := githubRelease{
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{
			{Name: "Kimmio-Launcher-1.2.0-linux-arm64.tar.gz", BrowserDownloadURL: "https://example/linux-arm64.tar.gz"},
			{Name: "Kimmio-Launcher-1.2.0-linux-arm64.deb", BrowserDownloadURL: "https://example/linux-arm64.deb"},
		},
	}

	if got := chooseLauncherAssetURL(release, "linux", "arm64"); got != "https://example/linux-arm64.deb" {
		t.Fatalf("linux arm64 should prefer deb over tar.gz, got %s", got)
	}
}
