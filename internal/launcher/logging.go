package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultLogMaxSizeBytes = 5 * 1024 * 1024
	defaultLogBackups      = 5
)

type structuredLogger struct {
	mu        sync.Mutex
	path      string
	maxSize   int64
	maxBackup int
}

var appLogger *structuredLogger

func initStructuredLogger(dataDir string) {
	path := filepath.Join(dataDir, "logs", "launcher.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log dir: %v\n", err)
		return
	}
	appLogger = &structuredLogger{path: path, maxSize: defaultLogMaxSizeBytes, maxBackup: defaultLogBackups}
}

func logInfo(msg string, fields map[string]any) {
	writeStructuredLog("INFO", msg, fields)
}

func logWarn(msg string, fields map[string]any) {
	writeStructuredLog("WARN", msg, fields)
}

func logError(msg string, fields map[string]any) {
	writeStructuredLog("ERROR", msg, fields)
}

func writeStructuredLog(level, msg string, fields map[string]any) {
	if appLogger == nil {
		return
	}
	appLogger.mu.Lock()
	defer appLogger.mu.Unlock()

	if err := appLogger.rotateIfNeeded(); err != nil {
		fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		return
	}

	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339),
		"level": level,
		"msg":   msg,
	}
	for k, v := range fields {
		record[k] = v
	}
	b, err := json.Marshal(record)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log marshal failed: %v\n", err)
		return
	}
	f, err := os.OpenFile(appLogger.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open log file failed: %v\n", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func (l *structuredLogger) rotateIfNeeded() error {
	st, err := os.Stat(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.Size() < l.maxSize {
		return nil
	}

	for i := l.maxBackup - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", l.path, i)
		dst := fmt.Sprintf("%s.%d", l.path, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Remove(dst)
			_ = os.Rename(src, dst)
		}
	}
	_ = os.Remove(l.path + ".1")
	return os.Rename(l.path, l.path+".1")
}
