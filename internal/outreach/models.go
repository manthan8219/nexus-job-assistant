package outreach

import "time"

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelLinkedIn Channel = "linkedin"
)

type Status string

const (
	StatusFinding  Status = "finding"  // pipeline: resolving HR/careers contact
	StatusDrafting Status = "drafting" // pipeline: AI writing/reviewing the email
	StatusDraft    Status = "draft"
	StatusReady    Status = "ready" // contact resolved, awaiting send
	StatusSent     Status = "sent"  // sent, follow-ups off or not applicable (LinkedIn)
	StatusFailed   Status = "failed"
	StatusSkipped  Status = "skipped"
	StatusOpened   Status = "opened" // browser launched (LinkedIn)
	// Follow-up sequence states (email channel).
	StatusFollowUpDue  Status = "followup_due"  // waiting for the next follow-up send time
	StatusSequenceDone Status = "sequence_done" // all follow-ups sent, no reply received
	StatusReplied      Status = "replied"       // human replied — sequence stopped
	StatusBounced      Status = "bounced"       // delivery rejected by the recipient server
)

// Item is one outreach attempt tied to a job application.
type Item struct {
	ID            string  `json:"id"`
	Channel       Channel `json:"channel"`
	JobURL        string  `json:"job_url"`
	Company       string  `json:"company"`
	Role          string  `json:"role"`
	Provider      string  `json:"provider,omitempty"`
	ContactName   string  `json:"contact_name,omitempty"`
	ContactEmail  string  `json:"contact_email,omitempty"`
	ContactTitle  string  `json:"contact_title,omitempty"`
	ContactSource string  `json:"contact_source,omitempty"` // hunter | apollo | github | osint | pattern | manual
	LinkedInURL   string  `json:"linkedin_url,omitempty"`
	Subject       string  `json:"subject,omitempty"`
	Body          string  `json:"body"`
	Status        Status  `json:"status"`
	Error         string  `json:"error,omitempty"`
	// Auto is true when the background pipeline created/processed this item.
	Auto bool `json:"auto,omitempty"`
	// AI review metadata (0 / empty when template-drafted or review disabled).
	ReviewScore int       `json:"review_score,omitempty"`
	ReviewNotes string    `json:"review_notes,omitempty"`
	Attempts    int       `json:"attempts,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SentAt      time.Time `json:"sent_at,omitempty"`
	// Follow-up sequence state (email only).
	// FollowUpStep counts messages sent: 0 = initial email, 1..MaxFollowUps.
	FollowUpStep int `json:"follow_up_step,omitempty"`
	// NextSendAt is when a StatusFollowUpDue item becomes actionable.
	NextSendAt time.Time `json:"next_send_at,omitempty"`
}

type StoreFile struct {
	Items []Item `json:"items"`
}
