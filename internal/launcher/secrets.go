package launcher

import (
	"os"
	"path/filepath"
	"strings"
)

func splitSecretEnv(env map[string]string) (map[string]string, map[string]string) {
	publicEnv := map[string]string{}
	secretEnv := map[string]string{}
	for k, v := range env {
		switch k {
		case "JWT_SECRET", "ENC_KEY_V0", "FLUMIO_ENC_KEY_V0":
			secretEnv[k] = v
		default:
			publicEnv[k] = v
		}
	}
	return publicEnv, secretEnv
}

func secretFilePath(profileID string) string {
	return filepath.Join(appCfg.DataDir, "secrets", profileID+".env")
}

func saveProfileSecrets(profileID string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(secretFilePath(profileID)), 0o700); err != nil {
		return err
	}
	lines := make([]string, 0, len(secrets))
	for k, v := range secrets {
		lines = append(lines, k+"="+strings.TrimSpace(v))
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(secretFilePath(profileID), []byte(content), 0o600)
}

func loadProfileSecrets(profileID string) map[string]string {
	result := map[string]string{}
	b, err := os.ReadFile(secretFilePath(profileID))
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k != "" {
			result[k] = v
		}
	}
	// Migrate legacy secret key name transparently on read.
	if strings.TrimSpace(result["ENC_KEY_V0"]) == "" && strings.TrimSpace(result["FLUMIO_ENC_KEY_V0"]) != "" {
		result["ENC_KEY_V0"] = strings.TrimSpace(result["FLUMIO_ENC_KEY_V0"])
	}
	delete(result, "FLUMIO_ENC_KEY_V0")
	return result
}
