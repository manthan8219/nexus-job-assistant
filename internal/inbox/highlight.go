package inbox

import "time"

// Signal classifies the hiring intent of an email.
type Signal string

const (
	SignalInterview   Signal = "interview"
	SignalRejection   Signal = "rejection"
	SignalOffer       Signal = "offer"
	SignalRecruiter   Signal = "recruiter"
	SignalApplication Signal = "application"
	SignalAssessment  Signal = "assessment"
	SignalNone        Signal = "none"
)

// Label returns a human-friendly label for a signal.
func (s Signal) Label() string {
	switch s {
	case SignalInterview:
		return "Interview"
	case SignalRejection:
		return "Rejection"
	case SignalOffer:
		return "Offer"
	case SignalRecruiter:
		return "Recruiter"
	case SignalApplication:
		return "Application"
	case SignalAssessment:
		return "Assessment"
	default:
		return "Other"
	}
}

// Highlight is one hiring-related email found by the inbox scan.
type Highlight struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id,omitempty"`
	From        string    `json:"from"`
	FromName    string    `json:"from_name,omitempty"`
	Subject     string    `json:"subject"`
	BodyPreview string    `json:"body_preview,omitempty"`
	Date        time.Time `json:"date"`
	Signal      Signal    `json:"signal"`
	Confidence  int       `json:"confidence"`
	Domain      string    `json:"domain,omitempty"`
	Company     string    `json:"company,omitempty"`
	AppID       int64     `json:"app_id,omitempty"`
	Seen        bool      `json:"seen,omitempty"`
}
