// Package settings persists user profile and consent overrides in a server-side
// relational store (Supabase/Postgres via pgx, or SQLite in tests) behind the
// database/sql interface. Secrets (the Gmail app password) are stored
// encrypted with AES-256-GCM; the in-DB key is itself wrapped by an env-var
// master key, so the column stays confidential even if the table leaks.
package settings

// ConfigOverrides is the user-editable subset of the application config that is
// persisted in the user_settings row (id=1). GmailAppPassword is always held
// and returned in plaintext in memory; it is encrypted at rest by Save.
//
// The zero value is meaningful: Load returns it when no row exists yet, which
// callers may treat as "no overrides configured".
type ConfigOverrides struct {
	FirstName         string
	LastName          string
	Email             string
	GmailAppPassword  string // always plaintext in-memory
	OutreachConsent   bool
	InboxScanMinutes  int
	City              string
	Phone             string
	LinkedInID        string
	TargetJobTitles   string
	WorkType          string
	TargetLocations   string
	Currency          string
	MinSalary         string
	ApplyConsent      bool
	MaxAppsPerRun     int
	MaxAppsPerDay     int
	ApplyDelaySec     int
	MinFitScore       int
	CompanyBlocklist  string
	NoticePeriodDays  string
	OfficeDaysPerWeek string
	CoverLetterMode   string
	WorkAuth          string
	ResumePath        string
}
