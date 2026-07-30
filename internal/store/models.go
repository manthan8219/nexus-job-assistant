package store

import "time"

type Status string

const (
	StatusApplied Status = "applied"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

// Outcome tracks what happened after an application was submitted.
// It lives in its own column so Status ("applied") and daily-cap counting
// are untouched — an applied job stays applied; its outcome evolves.
type Outcome string

const (
	OutcomeNone      Outcome = ""          // no response yet
	OutcomeReplied   Outcome = "replied"   // a human responded
	OutcomeInterview Outcome = "interview" // interview scheduled / in process
	OutcomeOffer     Outcome = "offer"     // offer received
	OutcomeRejected  Outcome = "rejected"  // explicit rejection
	OutcomeGhosted   Outcome = "ghosted"   // gave up waiting
)

// OutcomeCycle is the order the History tab cycles through with one keypress.
// Terminal-but-common outcomes (rejected, ghosted) come after the happy path.
var OutcomeCycle = []Outcome{
	OutcomeReplied,
	OutcomeInterview,
	OutcomeOffer,
	OutcomeRejected,
	OutcomeGhosted,
}

// NextOutcome returns the next outcome in the cycle; after the last one it
// wraps back to OutcomeNone (clearing the outcome).
func NextOutcome(cur Outcome) Outcome {
	for i, o := range OutcomeCycle {
		if o == cur {
			if i+1 < len(OutcomeCycle) {
				return OutcomeCycle[i+1]
			}
			return OutcomeNone
		}
	}
	return OutcomeCycle[0]
}

// ValidOutcome reports whether o is a settable outcome value.
func ValidOutcome(o Outcome) bool {
	if o == OutcomeNone {
		return true
	}
	for _, v := range OutcomeCycle {
		if o == v {
			return true
		}
	}
	return false
}

type Application struct {
	ID          int64
	Provider    string // "greenhouse", "lever", etc.
	Company     string
	Role        string
	URL         string
	Status      Status
	Reason      string // why skipped/failed, empty if applied
	AppliedAt   time.Time
	Location    string
	Remote      bool
	PostedAt    time.Time
	Description string
	FitScore    int    // 0-100 shortlist chance vs resume; 0 = unscored
	FitSummary  string // why the score is high/low
	// Outcome is the post-apply pipeline stage (empty = no response yet).
	Outcome   Outcome
	OutcomeAt time.Time // when the outcome was last set; zero when Outcome is empty
}
