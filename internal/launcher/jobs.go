package launcher

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type ActionJob struct {
	ID         string   `json:"id"`
	ProfileID  string   `json:"profileId"`
	Action     string   `json:"action"`
	Step       string   `json:"step,omitempty"`
	Status     string   `json:"status"`
	Message    string   `json:"message"`
	Progress   int      `json:"progress"`
	Error      string   `json:"error,omitempty"`
	Logs       []string `json:"logs,omitempty"`
	StartedAt  string   `json:"startedAt,omitempty"`
	FinishedAt string   `json:"finishedAt,omitempty"`
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/jobs/"))
	if jobID == "" {
		http.NotFound(w, r)
		return
	}

	s.jobMu.Lock()
	job, ok := s.jobs[jobID]
	if !ok {
		s.jobMu.Unlock()
		http.NotFound(w, r)
		return
	}
	copyJob := *job
	copyJob.Logs = append([]string{}, job.Logs...)
	s.jobMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"job": copyJob,
	})
}

func (s *Server) enqueueProfileJob(profileID, action string, run func(jobID string) error) (*ActionJob, error) {
	s.jobMu.Lock()
	if existingJobID, busy := s.activeProfiles[profileID]; busy {
		s.jobMu.Unlock()
		return nil, errors.New("another action is already running for this profile (job " + existingJobID + ")")
	}

	jobID := randomToken(16)
	job := &ActionJob{
		ID:        jobID,
		ProfileID: profileID,
		Action:    action,
		Status:    "queued",
		Message:   "Queued",
		Progress:  0,
		Logs:      []string{},
	}
	s.jobs[jobID] = job
	s.activeProfiles[profileID] = jobID
	s.jobMu.Unlock()

	go func() {
		s.updateJobStep(jobID, "prepare", "running", "Preparing action", 5, "")
		err := run(jobID)
		if err != nil {
			errText := err.Error()
			if strings.Contains(strings.ToLower(errText), "deadline exceeded") || strings.Contains(strings.ToLower(errText), "timeout") {
				s.updateJobStep(jobID, "cleanup", "timeout", "Timed out", 100, errText)
			} else {
				s.updateJobStep(jobID, "cleanup", "failed", "Failed", 100, errText)
			}
		} else {
			s.updateJobStep(jobID, "cleanup", "succeeded", "Completed", 100, "")
		}

		s.jobMu.Lock()
		delete(s.activeProfiles, profileID)
		s.jobMu.Unlock()
	}()

	return job, nil
}

func (s *Server) updateJob(jobID, status, message string, progress int, errText string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if status == "running" && job.StartedAt == "" {
		job.StartedAt = now
	}
	if status == "succeeded" || status == "failed" || status == "timeout" || status == "rolled_back" {
		job.FinishedAt = now
	}
	job.Status = status
	job.Message = message
	job.Progress = progress
	job.Error = errText
	if message != "" {
		job.Logs = append(job.Logs, now+" "+message)
		if len(job.Logs) > 100 {
			job.Logs = job.Logs[len(job.Logs)-100:]
		}
	}
}

func (s *Server) updateJobStep(jobID, step, status, message string, progress int, errText string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if status == "running" && job.StartedAt == "" {
		job.StartedAt = now
	}
	if status == "succeeded" || status == "failed" || status == "timeout" || status == "rolled_back" {
		job.FinishedAt = now
	}
	job.Step = step
	job.Status = status
	job.Message = message
	job.Progress = progress
	job.Error = errText
	if message != "" {
		job.Logs = append(job.Logs, now+" ["+step+"] "+message)
		if len(job.Logs) > 100 {
			job.Logs = job.Logs[len(job.Logs)-100:]
		}
	}
}
