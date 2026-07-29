package outreach

import "time"

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelLinkedIn Channel = "linkedin"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusReady    Status = "ready" // contact resolved, awaiting send
	StatusSent     Status = "sent"
	StatusFailed   Status = "failed"
	StatusSkipped  Status = "skipped"
	StatusOpened   Status = "opened" // browser launched (LinkedIn)
)

// Item is one outreach attempt tied to a job application.
type Item struct {
	ID           string    `json:"id"`
	Channel      Channel   `json:"channel"`
	JobURL       string    `json:"job_url"`
	Company      string    `json:"company"`
	Role         string    `json:"role"`
	Provider     string    `json:"provider,omitempty"`
	ContactName  string    `json:"contact_name,omitempty"`
	ContactEmail string    `json:"contact_email,omitempty"`
	ContactTitle string    `json:"contact_title,omitempty"`
	LinkedInURL  string    `json:"linkedin_url,omitempty"`
	Subject      string    `json:"subject,omitempty"`
	Body         string    `json:"body"`
	Status       Status    `json:"status"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	SentAt       time.Time `json:"sent_at,omitempty"`
}

type StoreFile struct {
	Items []Item `json:"items"`
}
