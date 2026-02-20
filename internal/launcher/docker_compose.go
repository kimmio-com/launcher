package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type composeProgressFn func(step, message string, progress int)

func (s *Server) performEnable(id, jobID string, parent context.Context) error {
	firstInstall := isFirstProfileInstall(id)
	actionTimeout := appCfg.EnableTimeout
	if actionTimeout < appCfg.ActionTimeout {
		actionTimeout = appCfg.ActionTimeout
	}

	ctx, cancel := context.WithTimeout(parent, actionTimeout)
	defer cancel()

	store, idx, err := s.getProfileForAction(id)
	if err != nil {
		return err
	}
	profile := store.Profiles[idx]

	logInfo("profile_enable_started", map[string]any{
		"profile_id":    id,
		"first_install": firstInstall,
		"timeout_sec":   int(actionTimeout.Seconds()),
		"version":       strings.TrimSpace(profile.Version),
	})

	if firstInstall {
		s.updateJobStep(jobID, "install", "running", "First-time setup detected. Installation can take up to 10 minutes.", 10, "")
	} else {
		s.updateJobStep(jobID, "up", "running", "Starting compose stack (non-destructive)", 30, "")
	}

	progress := func(step, message string, percent int) {
		s.updateJobStep(jobID, step, "running", message, percent, "")
		logInfo("profile_enable_progress", map[string]any{
			"profile_id": id,
			"step":       step,
			"progress":   percent,
			"message":    message,
		})
	}

	if err := runProfileComposeUp(ctx, profile, progress); err != nil {
		logError("profile_enable_failed", map[string]any{"profile_id": id, "error": err.Error()})
		_ = s.markProfileResult(id, "enable", "failed", err.Error(), "")
		return err
	}
	startingUntil := time.Now().UTC().Add(45 * time.Second).Format(time.RFC3339)
	if err := s.markProfileResult(id, "enable", "success", "Enable requested; waiting for health", startingUntil); err != nil {
		return err
	}
	s.updateJobStep(jobID, "health", "running", "Waiting for health", 85, "")
	if ok := waitForProfileHealthOrCanceled(ctx, profile, 6, 2*time.Second); !ok {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		logWarn("profile_enable_health_pending", map[string]any{"profile_id": id})
		_ = s.markProfileResult(id, "enable", "warning", "Instance did not become healthy yet", startingUntil)
		return nil
	}
	logInfo("profile_enable_succeeded", map[string]any{"profile_id": id})
	return s.markProfileResult(id, "enable", "success", "Instance is healthy", "")
}

func (s *Server) performStop(id, jobID string, parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, appCfg.ActionTimeout)
	defer cancel()

	s.updateJobStep(jobID, "down", "running", "Stopping compose stack", 35, "")
	if err := runProfileComposeDown(ctx, id, false); err != nil {
		_ = s.markProfileResult(id, "stop", "failed", err.Error(), "")
		return err
	}
	return s.markProfileResult(id, "stop", "success", "Profile stopped", "")
}

func (s *Server) performRecreate(id, jobID string, parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, appCfg.ActionTimeout)
	defer cancel()

	store, idx, err := s.getProfileForAction(id)
	if err != nil {
		return err
	}
	profile := store.Profiles[idx]

	s.updateJobStep(jobID, "down", "running", "Resetting stack and volumes", 30, "")
	if err := runProfileComposeDown(ctx, id, true); err != nil {
		_ = s.markProfileResult(id, "recreate", "failed", err.Error(), "")
		return err
	}
	s.updateJobStep(jobID, "up", "running", "Starting fresh stack", 60, "")
	if err := runProfileComposeUp(ctx, profile, func(step, message string, progress int) {
		s.updateJobStep(jobID, step, "running", message, progress, "")
	}); err != nil {
		_ = s.markProfileResult(id, "recreate", "failed", err.Error(), "")
		return err
	}
	startingUntil := time.Now().UTC().Add(45 * time.Second).Format(time.RFC3339)
	if err := s.markProfileResult(id, "recreate", "success", "Recreate requested; waiting for health", startingUntil); err != nil {
		return err
	}
	if ok := waitForProfileHealthOrCanceled(ctx, profile, 6, 2*time.Second); !ok {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		_ = s.markProfileResult(id, "recreate", "warning", "Instance did not become healthy yet", startingUntil)
		return nil
	}
	return s.markProfileResult(id, "recreate", "success", "Instance is healthy", "")
}

func (s *Server) performDelete(id, jobID string, parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, appCfg.ActionTimeout)
	defer cancel()

	s.mu.Lock()
	store, err := loadProfileStore(s.dbPath)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	idx := findProfileIndex(store, id)
	if idx < 0 {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	s.mu.Unlock()

	s.updateJobStep(jobID, "cleanup", "running", "Removing stack and volumes", 45, "")
	if err := runProfileComposeDown(ctx, id, true); err != nil {
		return err
	}

	s.mu.Lock()
	store, err = loadProfileStore(s.dbPath)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	idx = findProfileIndex(store, id)
	if idx < 0 {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	store.Profiles = append(store.Profiles[:idx], store.Profiles[idx+1:]...)
	err = writeProfileStoreAtomic(s.dbPath, store)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	_ = os.RemoveAll(profileComposeDir(id))
	_ = os.Remove(secretFilePath(id))
	return nil
}

func (s *Server) performVersionUpdate(id, newVersion, jobID string, parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, appCfg.ActionTimeout)
	defer cancel()

	s.mu.Lock()
	store, err := loadProfileStore(s.dbPath)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	idx := findProfileIndex(store, id)
	if idx < 0 {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	oldProfile := store.Profiles[idx]
	oldVersion := oldProfile.Version
	store.Profiles[idx].Version = newVersion
	store.Profiles[idx].LastRequestedVersion = newVersion
	err = writeProfileStoreAtomic(s.dbPath, store)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	if !oldProfile.Enabled {
		return s.markProfileResult(id, "version", "success", "Version updated to "+newVersion, "")
	}

	s.updateJobStep(jobID, "up", "running", "Rebuilding with new version", 45, "")
	newProfile := oldProfile
	newProfile.Version = newVersion
	if err := runProfileComposeUp(ctx, newProfile, nil); err != nil {
		s.updateJobStep(jobID, "cleanup", "running", "Rolling back to previous version", 75, "")
		rollbackErr := runProfileComposeUp(ctx, oldProfile, nil)
		_ = s.restoreVersion(id, oldVersion, rollbackErr == nil)
		if rollbackErr != nil {
			return fmt.Errorf("update failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("update failed and rolled back: %w", err)
	}
	return s.markProfileResult(id, "version", "success", "Version updated to "+newVersion, "")
}

func (s *Server) performRegenerateSecrets(id, jobID string, parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, appCfg.ActionTimeout)
	defer cancel()

	store, idx, err := s.getProfileForAction(id)
	if err != nil {
		return err
	}
	profile := store.Profiles[idx]

	newSecrets := map[string]string{
		"JWT_SECRET":        randomToken(48),
		"FLUMIO_ENC_KEY_V0": randomToken(32),
	}
	if err := saveProfileSecrets(id, newSecrets); err != nil {
		_ = s.markProfileResult(id, "regenerate-secrets", "failed", err.Error(), "")
		return err
	}

	if !profile.Enabled {
		return s.markProfileResult(id, "regenerate-secrets", "success", "Secrets regenerated", "")
	}

	s.updateJobStep(jobID, "up", "running", "Applying regenerated secrets", 50, "")
	if err := runProfileComposeUp(ctx, profile, nil); err != nil {
		_ = s.markProfileResult(id, "regenerate-secrets", "failed", err.Error(), "")
		return err
	}
	return s.markProfileResult(id, "regenerate-secrets", "success", "Secrets regenerated and applied", "")
}

func runProfileComposeUp(ctx context.Context, profile ProfileRequest, onProgress composeProgressFn) error {
	notify := func(step, message string, progress int) {
		if onProgress != nil {
			onProgress(step, message, progress)
		}
	}

	notify("prepare", "Preparing compose files", 18)
	composeDir := profileComposeDir(profile.ID)
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(composeDir, "compose.yaml"), []byte(buildComposeYAML()), 0o644); err != nil {
		return err
	}

	envContent := buildComposeEnv(profile)
	if err := os.WriteFile(filepath.Join(composeDir, ".env"), []byte(envContent), 0o644); err != nil {
		return err
	}

	project := dockerProjectName(profile.ID)
	dockerBin, err := dockerBinaryPath()
	if err != nil {
		return err
	}

	image := "kimmio/kimmio-app:" + strings.TrimSpace(profile.Version)
	if strings.TrimSpace(profile.Version) == "" {
		image = "kimmio/kimmio-app:latest"
	}
	notify("pull", "Pulling Docker image "+image+" (can take several minutes)", 30)
	if err := pullImageWithRetry(ctx, dockerBin, image, 3, func(attempt, attempts int) {
		if attempts <= 1 {
			notify("pull", "Pulling Docker image "+image, 30)
			return
		}
		notify("pull", fmt.Sprintf("Pulling Docker image %s (attempt %d/%d)", image, attempt, attempts), 30+(attempt-1)*5)
	}); err != nil {
		return err
	}

	notify("up", "Starting containers", 60)
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := dockerCommandWithContext(ctx, dockerBin, "compose", "-p", project, "-f", "compose.yaml", "up", "-d", "--build")
		cmd.Dir = composeDir
		out, err := cmd.CombinedOutput()
		if err == nil {
			logInfo("compose_up_succeeded", map[string]any{
				"profile_id": profile.ID,
				"attempt":    attempt,
				"project":    project,
			})
			if attempt > 1 {
				logInfo("compose_up_retry_succeeded", map[string]any{"profile_id": profile.ID, "attempt": attempt})
			}
			notify("up", "Containers started; validating health", 78)
			return nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		notify("up", fmt.Sprintf("Container startup failed (attempt %d/3), retrying", attempt), 60+attempt*5)
		logWarn("compose_up_attempt_failed", map[string]any{
			"profile_id": profile.ID,
			"attempt":    attempt,
			"error":      strings.TrimSpace(string(out)),
		})
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("%s", friendlyDockerError(lastErr.Error()))
	}
	return fmt.Errorf("failed to start compose stack")
}

func waitForProfileHealthOrCanceled(ctx context.Context, profile ProfileRequest, attempts int, sleep time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if isProfileHealthy(profile) {
			return true
		}
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(sleep):
			}
		}
	}
	return false
}

func runProfileComposeDown(ctx context.Context, id string, removeVolumes bool) error {
	composeDir := profileComposeDir(id)
	if _, err := os.Stat(filepath.Join(composeDir, "compose.yaml")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	args := []string{"compose", "-p", dockerProjectName(id), "-f", "compose.yaml", "down"}
	if removeVolumes {
		args = append(args, "--volumes", "--remove-orphans")
	}
	dockerBin, err := dockerBinaryPath()
	if err != nil {
		return err
	}
	cmd := dockerCommandWithContext(ctx, dockerBin, args...)
	cmd.Dir = composeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pullImageWithRetry(ctx context.Context, dockerBin, image string, attempts int, onAttempt func(attempt, attempts int)) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if onAttempt != nil {
			onAttempt(attempt, attempts)
		}
		logInfo("docker_pull_started", map[string]any{
			"image":   image,
			"attempt": attempt,
			"total":   attempts,
		})
		cmd := dockerCommandWithContext(ctx, dockerBin, "pull", image)
		out, err := cmd.CombinedOutput()
		if err == nil {
			logInfo("docker_pull_succeeded", map[string]any{
				"image":   image,
				"attempt": attempt,
			})
			return nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		logWarn("docker_pull_attempt_failed", map[string]any{
			"image":   image,
			"attempt": attempt,
			"error":   strings.TrimSpace(string(out)),
		})
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("%s", friendlyDockerError(lastErr.Error()))
	}
	return fmt.Errorf("failed to pull image")
}

func isFirstProfileInstall(profileID string) bool {
	composeFile := filepath.Join(profileComposeDir(profileID), "compose.yaml")
	_, err := os.Stat(composeFile)
	return errors.Is(err, os.ErrNotExist)
}

func friendlyDockerError(raw string) string {
	msg := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(msg, "cannot connect to the docker daemon"):
		return "Docker daemon is not reachable. Start Docker Desktop (or Docker service) and try again."
	case strings.Contains(msg, "pull access denied"), strings.Contains(msg, "manifest unknown"), strings.Contains(msg, "not found"):
		return "Unable to pull Kimmio image tag. Verify the selected version exists and try again."
	case strings.Contains(msg, "port is already allocated"), strings.Contains(msg, "address already in use"):
		return "Host port is already in use by another process. Choose another profile port."
	case strings.Contains(msg, "no space left on device"):
		return "Not enough disk space for Docker image/containers. Free up space and retry."
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "timeout"):
		return "Docker operation timed out while pulling or starting containers. Retry after checking network and Docker health."
	default:
		return "Docker failed to start this profile. Check Docker Desktop status and logs, then retry."
	}
}

func profileComposeDir(id string) string {
	return filepath.Join(appCfg.DataDir, "compose", id)
}

func dockerProjectName(id string) string {
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, strings.ToLower(id))
	return "kimmio-" + strings.Trim(clean, "-")
}

func buildComposeYAML() string {
	return `services:
  kimmio_app:
    image: ${KIMMIO_APP_IMAGE}
    restart: always
    depends_on:
      - postgres
      - redis
      - minio
    environment:
      JWT_SECRET: ${JWT_SECRET}
      FLUMIO_ENC_KEY_V0: ${FLUMIO_ENC_KEY_V0}
      INSTANCE_ID: ${INSTANCE_ID}
      PORT: ${APP_PORT}
      DOMAIN: ${APP_DOMAIN}
      WEBSOCKET_PORT: ${WEBSOCKET_PORT}
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
      MINIO_ROOT_HOST: ${MINIO_ROOT_HOST}
      MINIO_ROOT_PORT: ${MINIO_ROOT_PORT}
      REDIS_PASSWORD: ${REDIS_PASSWORD}
      REDIS_PORT: ${REDIS_PORT}
      REDIS_HOST: ${REDIS_HOST}
      POSTGRES_HOST: ${POSTGRES_HOST}
      POSTGRES_PORT: ${POSTGRES_PORT}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    ports:
      - "${APP_PORT}:${APP_PORT}"
    networks:
      - public
      - internal
    volumes:
      - kimmio_data:/app/.data
      - kimmio_run:/app/.run
    healthcheck:
      test: [ "CMD", "wget", "-qO-", "http://localhost:$${APP_PORT}/health" ]
      interval: 30s
      timeout: 5s
      retries: 5
    deploy:
      resources:
        limits:
          cpus: "${CPU_LIMIT}"
          memory: ${MEMORY_LIMIT}
        reservations:
          cpus: "0.25"
          memory: 256M

  postgres:
    image: pgvector/pgvector:pg16
    restart: always
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    networks:
      - internal
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: [ "CMD-SHELL", "pg_isready -U $${POSTGRES_USER}" ]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7.2
    restart: always
    command: >
      redis-server
      --appendonly yes
      --requirepass ${REDIS_PASSWORD}
    networks:
      - internal
    volumes:
      - redis_data:/data
    healthcheck:
      test: [ "CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping" ]
      interval: 10s
      timeout: 3s
      retries: 5

  minio:
    image: minio/minio:RELEASE.2024-01-31T20-20-33Z
    restart: always
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    networks:
      - internal
    volumes:
      - minio_data:/data
    healthcheck:
      test: [ "CMD", "curl", "-f", "http://localhost:9000/minio/health/live" ]
      interval: 30s
      timeout: 5s
      retries: 5

networks:
  public:
    driver: bridge
  internal:
    driver: bridge
    internal: true

volumes:
  postgres_data:
    name: ${INSTANCE_ID}_postgres_data
  redis_data:
    name: ${INSTANCE_ID}_redis_data
  kimmio_data:
    name: ${INSTANCE_ID}_kimmio_data
  kimmio_run:
    name: ${INSTANCE_ID}_kimmio_run
  minio_data:
    name: ${INSTANCE_ID}_minio_data
`
}

func buildComposeEnv(profile ProfileRequest) string {
	hostPort := 8080
	if len(profile.Ports) > 0 && profile.Ports[0].Host > 0 {
		hostPort = profile.Ports[0].Host
	}

	version := strings.TrimSpace(profile.Version)
	if version == "" {
		version = "latest"
	}

	mem := strings.TrimSpace(profile.Resources.Limits.Memory)
	if mem == "" {
		mem = "4024M"
	}

	cpus := profile.Resources.Limits.CPUs
	if cpus <= 0 {
		cpus = 1.0
	}

	base := strings.ReplaceAll(profile.ID, "-", "_")
	mergedEnv := map[string]string{}
	for k, v := range profile.Env {
		mergedEnv[k] = v
	}
	for k, v := range loadProfileSecrets(profile.ID) {
		mergedEnv[k] = v
	}
	jwtSecret := strings.TrimSpace(envValue(mergedEnv, "JWT_SECRET", ""))
	if len(jwtSecret) < 32 {
		if jwtSecret != "" {
			logWarn("invalid_secret_length_autoheal", map[string]any{"profile_id": profile.ID, "secret": "JWT_SECRET", "length": len(jwtSecret)})
		}
		jwtSecret = randomToken(48)
	}
	flumioKey := strings.TrimSpace(envValue(mergedEnv, "FLUMIO_ENC_KEY_V0", ""))
	if len(flumioKey) != 32 {
		if flumioKey != "" {
			logWarn("invalid_secret_length_autoheal", map[string]any{"profile_id": profile.ID, "secret": "FLUMIO_ENC_KEY_V0", "length": len(flumioKey)})
		}
		flumioKey = randomToken(32)
	}
	lines := []string{
		"JWT_SECRET=" + jwtSecret,
		"FLUMIO_ENC_KEY_V0=" + flumioKey,
		"INSTANCE_ID=" + envValue(mergedEnv, "INSTANCE_ID", profile.ID),
		"APP_PORT=" + envValue(mergedEnv, "APP_PORT", strconv.Itoa(hostPort)),
		"APP_DOMAIN=" + envValue(mergedEnv, "APP_DOMAIN", "localhost"),
		"WEBSOCKET_PORT=" + envValue(mergedEnv, "WEBSOCKET_PORT", strconv.Itoa(hostPort)),
		"KIMMIO_APP_IMAGE=kimmio/kimmio-app:" + version,
		"POSTGRES_USER=" + envValue(mergedEnv, "POSTGRES_USER", "postgres"),
		"POSTGRES_PASSWORD=" + envValue(mergedEnv, "POSTGRES_PASSWORD", "postgres"),
		"POSTGRES_HOST=" + envValue(mergedEnv, "POSTGRES_HOST", "postgres"),
		"POSTGRES_DB=" + envValue(mergedEnv, "POSTGRES_DB", profile.ID),
		"POSTGRES_PORT=" + envValue(mergedEnv, "POSTGRES_PORT", "5432"),
		"REDIS_HOST=" + envValue(mergedEnv, "REDIS_HOST", "redis"),
		"REDIS_PORT=" + envValue(mergedEnv, "REDIS_PORT", "6379"),
		"REDIS_PASSWORD=" + envValue(mergedEnv, "REDIS_PASSWORD", profile.ID+"_redis_pw"),
		"MINIO_ROOT_USER=" + envValue(mergedEnv, "MINIO_ROOT_USER", "minio_"+base),
		"MINIO_ROOT_PASSWORD=" + envValue(mergedEnv, "MINIO_ROOT_PASSWORD", profile.ID+"_minio_pw"),
		"MINIO_ROOT_HOST=" + envValue(mergedEnv, "MINIO_ROOT_HOST", "minio"),
		"MINIO_ROOT_PORT=" + envValue(mergedEnv, "MINIO_ROOT_PORT", "9000"),
		"MEMORY_LIMIT=" + mem,
		"CPU_LIMIT=" + fmt.Sprintf("%.2f", cpus),
	}

	return strings.Join(lines, "\n") + "\n"
}

func profileEnvValue(profile ProfileRequest, key, fallback string) string {
	if profile.Env == nil {
		return fallback
	}
	if v, ok := profile.Env[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func envValue(values map[string]string, key, fallback string) string {
	if values == nil {
		return fallback
	}
	if v, ok := values[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func waitForProfileHealth(profile ProfileRequest, attempts int, sleep time.Duration) bool {
	if attempts <= 0 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if isProfileHealthy(profile) {
			return true
		}
		time.Sleep(sleep)
	}
	return false
}
