package launcher

import (
	"launcher/internal/config"
	"testing"
	"time"
)

func TestEnqueueProfileJobLocksByProfile(t *testing.T) {
	cfg := config.Load("dev")
	appCfg = cfg
	srv := NewServer(cfg)
	done := make(chan struct{})

	job1, err := srv.enqueueProfileJob("kimmio-default", "enable", func(jobID string) error {
		<-done
		return nil
	})
	if err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	if job1 == nil || job1.ID == "" {
		t.Fatalf("expected first job with id")
	}

	_, err = srv.enqueueProfileJob("kimmio-default", "stop", func(jobID string) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected lock error for second job on same profile")
	}

	close(done)
	time.Sleep(80 * time.Millisecond)

	srv.jobMu.Lock()
	defer srv.jobMu.Unlock()
	stored := srv.jobs[job1.ID]
	if stored == nil {
		t.Fatalf("expected job stored")
	}
	if stored.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q", stored.Status)
	}
}
