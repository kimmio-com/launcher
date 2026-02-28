package launcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"launcher/internal/config"
)

func RunCLI(cfg config.Config, args []string, stdout, stderr io.Writer) (handled bool, exitCode int) {
	args = normalizeCLIArgs(args)
	if len(args) == 0 {
		return false, 0
	}
	if strings.ToLower(strings.TrimSpace(args[0])) != "profile" {
		return false, 0
	}

	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	appCfg = cfg
	srv := NewServer(cfg)
	return true, runProfileCLI(srv, args[1:], stdout, stderr)
}

func normalizeCLIArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if strings.TrimSpace(args[0]) == "--" {
		return args[1:]
	}
	return args
}

func runProfileCLI(srv *Server, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeProfileCLIUsage(stderr)
		return 2
	}

	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "help", "-h", "--help":
		writeProfileCLIUsage(stdout)
		return 0
	case "list":
		if len(args) != 1 {
			writeProfileCLIUsage(stderr)
			return 2
		}
		return runProfileList(srv, stdout, stderr)
	}

	if len(args) < 2 {
		writeProfileCLIUsage(stderr)
		return 2
	}

	profileID := strings.ToLower(strings.TrimSpace(args[0]))
	action := strings.ToLower(strings.TrimSpace(args[1]))
	switch action {
	case "info":
		if len(args) != 2 {
			writeProfileCLIUsage(stderr)
			return 2
		}
		return runProfileInfo(srv, profileID, stdout, stderr)
	case "update":
		version := "latest"
		if len(args) > 3 {
			writeProfileCLIUsage(stderr)
			return 2
		}
		if len(args) == 3 {
			version = strings.TrimSpace(args[2])
		}
		return runProfileUpdate(srv, profileID, version, stdout, stderr)
	case "delete":
		if len(args) != 2 {
			writeProfileCLIUsage(stderr)
			return 2
		}
		return runProfileDelete(srv, profileID, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Unknown profile action: %s\n", action)
		writeProfileCLIUsage(stderr)
		return 2
	}
}

func runProfileList(srv *Server, stdout, stderr io.Writer) int {
	store, err := loadProfileStore(srv.dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "Failed to load profiles: %v\n", err)
		return 1
	}
	if len(store.Profiles) == 0 {
		fmt.Fprintln(stdout, "No profiles found.")
		return 0
	}

	profiles := applyHealthStatus(store.Profiles)
	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tVERSION\tPORT\tSTATUS\tENABLED")
	for _, p := range profiles {
		port := 0
		if len(p.Ports) > 0 {
			port = p.Ports[0].Host
		}
		status := strings.TrimSpace(p.RuntimeStatus)
		if status == "" {
			status = "unknown"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%t\n", p.ID, p.Version, port, status, p.Enabled)
	}
	_ = tw.Flush()
	return 0
}

func runProfileInfo(srv *Server, profileID string, stdout, stderr io.Writer) int {
	if !profileIDRe.MatchString(profileID) {
		fmt.Fprintf(stderr, "Invalid profile name: %s\n", profileID)
		return 2
	}

	store, err := loadProfileStore(srv.dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "Failed to load profiles: %v\n", err)
		return 1
	}
	profiles := applyHealthStatus(store.Profiles)
	idx := findProfileIndex(ProfileStore{Profiles: profiles}, profileID)
	if idx < 0 {
		fmt.Fprintf(stderr, "Profile not found: %s\n", profileID)
		return 1
	}

	p := profiles[idx]
	port := 0
	if len(p.Ports) > 0 {
		port = p.Ports[0].Host
	}
	domain := strings.TrimSpace(p.Env["APP_DOMAIN"])
	if domain == "" {
		domain = "localhost"
	}

	fmt.Fprintf(stdout, "ID: %s\n", p.ID)
	fmt.Fprintf(stdout, "Version: %s\n", p.Version)
	fmt.Fprintf(stdout, "Host Port: %d\n", port)
	fmt.Fprintf(stdout, "Domain: %s\n", domain)
	fmt.Fprintf(stdout, "Enabled: %t\n", p.Enabled)
	fmt.Fprintf(stdout, "Running: %t\n", p.Running)
	fmt.Fprintf(stdout, "Runtime Status: %s\n", p.RuntimeStatus)
	if p.LastAction != "" {
		fmt.Fprintf(stdout, "Last Action: %s\n", p.LastAction)
	}
	if p.LastActionStatus != "" {
		fmt.Fprintf(stdout, "Last Action Status: %s\n", p.LastActionStatus)
	}
	if p.LastActionResult != "" {
		fmt.Fprintf(stdout, "Last Action Result: %s\n", p.LastActionResult)
	}
	if p.LastActionAt != "" {
		fmt.Fprintf(stdout, "Last Action At: %s\n", p.LastActionAt)
	}
	return 0
}

func runProfileUpdate(srv *Server, profileID, version string, stdout, stderr io.Writer) int {
	if !profileIDRe.MatchString(profileID) {
		fmt.Fprintf(stderr, "Invalid profile name: %s\n", profileID)
		return 2
	}
	version = strings.TrimSpace(version)
	if version == "" {
		version = "latest"
	}
	if !versionTagRe.MatchString(version) {
		fmt.Fprintf(stderr, "Invalid version tag: %s\n", version)
		return 2
	}
	if _, _, err := srv.getProfileForAction(profileID); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(stderr, "Profile not found: %s\n", profileID)
			return 1
		}
		fmt.Fprintf(stderr, "Failed to load profile: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Updating profile %s to version %s...\n", profileID, version)
	if err := srv.performVersionUpdate(profileID, version, "", context.Background()); err != nil {
		fmt.Fprintf(stderr, "Update failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %s updated to version %s.\n", profileID, version)
	return 0
}

func runProfileDelete(srv *Server, profileID string, stdout, stderr io.Writer) int {
	if !profileIDRe.MatchString(profileID) {
		fmt.Fprintf(stderr, "Invalid profile name: %s\n", profileID)
		return 2
	}

	fmt.Fprintf(stdout, "Deleting profile %s...\n", profileID)
	if err := srv.performDelete(profileID, "", context.Background()); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(stderr, "Profile not found: %s\n", profileID)
			return 1
		}
		fmt.Fprintf(stderr, "Delete failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Profile %s deleted.\n", profileID)
	return 0
}

func writeProfileCLIUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  profile list")
	fmt.Fprintln(w, "  profile <name> info")
	fmt.Fprintln(w, "  profile <name> update [version]")
	fmt.Fprintln(w, "  profile <name> delete")
}
