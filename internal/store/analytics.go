package store

import (
	"sort"
	"time"
)

// AnalyticsSnapshot is the full aggregation the /api/analytics endpoint serves.
type AnalyticsSnapshot struct {
	// StatusTotals counts every application row by status (applied/skipped/failed/queued).
	StatusTotals map[string]int `json:"statusTotals"`
	// Funnel counts applications by their latest outcome.
	Funnel Funnel `json:"funnel"`
	// PerProvider lists each provider's application funnel, ordered by provider name.
	PerProvider []ProviderYield `json:"perProvider"`
	// AppliedLast7Days is daily applied counts for the last 7 calendar days, oldest first.
	AppliedLast7Days []DayCount `json:"appliedLast7Days"`
	// AppliedLast30Days is daily applied counts for the last 30 calendar days, oldest first.
	AppliedLast30Days []DayCount `json:"appliedLast30Days"`
	// ResponseProbability is the overall 0-100 likelihood of getting any human
	// response (replied, interview or offer) across all applications (KAN-19).
	ResponseProbability int `json:"responseProbability"`
	// GeneratedAt is when the snapshot was computed.
	GeneratedAt time.Time `json:"generatedAt"`
}

// Funnel is the outcome funnel: applied → replied → interview → offer,
// plus the terminal rejected and ghosted states.
type Funnel struct {
	Applied   int `json:"applied"`
	Replied   int `json:"replied"`
	Interview int `json:"interview"`
	Offer     int `json:"offer"`
	Rejected  int `json:"rejected"`
	Ghosted   int `json:"ghosted"`
}

// ProviderYield is one provider's funnel counts.
type ProviderYield struct {
	Provider  string `json:"provider"`
	Applied   int    `json:"applied"`
	Replied   int    `json:"replied"`
	Interview int    `json:"interview"`
	Offer     int    `json:"offer"`
	// ReplyProbability is the 0-100 likelihood of a response from this
	// provider, derived from its funnel counts (KAN-19).
	ReplyProbability int `json:"replyProbability"`
}

// replyProbability scores how likely a human response is given a provider's
// funnel: any replied, interview or offer outcome counts as a response.
// Zero applied (or no data) scores 0; the result is clamped to 0-100.
func replyProbability(applied, replied, interview, offer int) int {
	if applied <= 0 {
		return 0
	}
	responses := replied + interview + offer
	p := 100 * responses / applied
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// DayCount is the number of applications recorded on one calendar day.
type DayCount struct {
	Date  string `json:"date"` // YYYY-MM-DD in local time
	Count int    `json:"count"`
}

// AnalyticsSnapshot aggregates the applications table into one snapshot for
// the analytics dashboard. An empty database yields zeroed, non-nil maps and
// slices so the JSON response shape stays stable.
func (s *Store) AnalyticsSnapshot() (*AnalyticsSnapshot, error) {
	snap := &AnalyticsSnapshot{
		StatusTotals:      map[string]int{},
		PerProvider:       []ProviderYield{},
		AppliedLast7Days:  []DayCount{},
		AppliedLast30Days: []DayCount{},
		GeneratedAt:       time.Now(),
	}

	rows, err := s.db.Query(`SELECT provider, status, outcome, applied_at FROM applications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	provider := map[string]*ProviderYield{}
	byDay := map[string]int{}

	for rows.Next() {
		var providerName, status, outcome string
		var appliedAt time.Time
		if err := rows.Scan(&providerName, &status, &outcome, &appliedAt); err != nil {
			return nil, err
		}
		snap.StatusTotals[status]++

		py, ok := provider[providerName]
		if !ok {
			py = &ProviderYield{Provider: providerName}
			provider[providerName] = py
		}
		if status == string(StatusApplied) {
			py.Applied++
			snap.Funnel.Applied++
		}
		switch Outcome(outcome) {
		case OutcomeReplied:
			py.Replied++
			snap.Funnel.Replied++
		case OutcomeInterview:
			py.Interview++
			snap.Funnel.Interview++
		case OutcomeOffer:
			py.Offer++
			snap.Funnel.Offer++
		case OutcomeRejected:
			snap.Funnel.Rejected++
		case OutcomeGhosted:
			snap.Funnel.Ghosted++
		}

		// Bucket applied applications by their local calendar day, keeping only
		// the last 30 days so the map stays small.
		if status == string(StatusApplied) && !appliedAt.IsZero() {
			local := appliedAt.In(now.Location())
			day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, now.Location())
			if !day.Before(dayStart.AddDate(0, 0, -29)) && !day.After(dayStart) {
				byDay[day.Format("2006-01-02")]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Deterministic per-provider ordering.
	names := make([]string, 0, len(provider))
	for name := range provider {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		py := provider[name]
		py.ReplyProbability = replyProbability(py.Applied, py.Replied, py.Interview, py.Offer)
		snap.PerProvider = append(snap.PerProvider, *py)
	}
	snap.ResponseProbability = replyProbability(snap.Funnel.Applied, snap.Funnel.Replied, snap.Funnel.Interview, snap.Funnel.Offer)

	snap.AppliedLast7Days = dayBuckets(byDay, dayStart, 7)
	snap.AppliedLast30Days = dayBuckets(byDay, dayStart, 30)
	return snap, nil
}

// dayBuckets fills the last n calendar days (oldest first) from byDay.
func dayBuckets(byDay map[string]int, dayStart time.Time, n int) []DayCount {
	out := make([]DayCount, 0, n)
	for i := n - 1; i >= 0; i-- {
		d := dayStart.AddDate(0, 0, -i)
		date := d.Format("2006-01-02")
		out = append(out, DayCount{Date: date, Count: byDay[date]})
	}
	return out
}
