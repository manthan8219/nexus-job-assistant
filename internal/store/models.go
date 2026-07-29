package store

import "time"

type Status string

const (
	StatusApplied Status = "applied"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

type Application struct {
	ID          int64
	Provider    string    // "greenhouse", "lever", etc.
	Company     string
	Role        string
	URL         string
	Status      Status
	Reason      string    // why skipped/failed, empty if applied
	AppliedAt   time.Time
	Location    string
	Remote      bool
	PostedAt    time.Time
	Description string
	FitScore    int    // 0-100 shortlist chance vs resume; 0 = unscored
	FitSummary  string // why the score is high/low
}
