package outreach

import "time"

// warmupCap returns the day's send cap during a sender warm-up ramp. rampDays
// <= 0 disables the ramp and returns maxCap unchanged. The cap grows linearly
// from day one up to the full daily cap over rampDays active sending days, so a
// fresh sender domain is not blasted at full volume on day one.
func warmupCap(daysActive, rampDays, maxCap int) int {
	if rampDays <= 0 || maxCap <= 0 {
		return maxCap
	}
	if daysActive < 1 {
		daysActive = 1
	}
	if daysActive >= rampDays {
		return maxCap
	}
	c := maxCap * daysActive / rampDays
	if c < 1 {
		c = 1
	}
	return c
}

// sendingDaysActive returns how many distinct days the sender has been active,
// counting from the earliest sent email to now (1-based). Zero sent items
// means the sender is on day one.
func sendingDaysActive(items []Item, now time.Time) int {
	var first time.Time
	for _, it := range items {
		if it.Channel != ChannelEmail || it.SentAt.IsZero() {
			continue
		}
		if first.IsZero() || it.SentAt.Before(first) {
			first = it.SentAt
		}
	}
	if first.IsZero() {
		return 1
	}
	days := int(now.Sub(first).Hours()/24) + 1
	if days < 1 {
		days = 1
	}
	return days
}
