package inbox

import "strings"

// hiringRule pairs a signal with the phrases that indicate it. The classifier
// walks rules in order - the first match wins, so ordering encodes priority
// (rejection before offer before interview, etc.).
type hiringRule struct {
	signal  Signal
	phrases []string
}

var hiringRules = []hiringRule{
	{SignalRejection, []string{
		"unfortunately", "regret to inform", "not moving forward", "not been selected",
		"not selected", "other candidates", "position has been filled", "decided not to proceed",
		"unable to move forward", "no longer be considered", "will not be moving forward",
		"not proceeding", "after careful consideration",
	}},
	{SignalInterview, []string{
		"interview", "phone screen", "technical screen", "screening call", "schedule a time",
		"book a time", "next steps", "we'd love to", "would love to", "let's chat",
		"lets chat", "call with", "meet with", "assessment day", "onsite interview",
		"virtual meeting", "calendar invite", "availability",
	}},
	{SignalOffer, []string{
		"offer letter", "job offer", "we'd like to offer", "we are pleased to offer",
		"pleased to offer", "extend an offer", "to offer you the position", "congratulations",
		"welcome to the team", "acceptance letter", "signed offer",
	}},
	{SignalAssessment, []string{
		"coding assessment", "coding challenge", "take home", "take-home", "code challenge",
		"hackerrank", "codility", "technical test", "technical assignment", "take-home assignment",
	}},
	{SignalRecruiter, []string{
		"recruiter", "found your profile", "came across your profile", "your profile",
		"we are hiring", "we're hiring", "opportunity", "open position", "would you be interested",
		"headhunter", "talent acquisition", "reaching out about", "reach out about",
	}},
	{SignalApplication, []string{
		"thank you for applying", "application received", "we received your application",
		"received your application", "your application", "application for", "thank you for your interest",
	}},
}

// textMatches reports whether any phrase appears in the lowercased text.
func textMatches(text string, phrases []string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// Classify returns the hiring signal for an email and a 0-100 confidence.
// Subject matches are trusted more than body-only matches.
func Classify(subject, body string) (Signal, int) {
	sub := strings.ToLower(subject)
	all := strings.ToLower(body)
	for _, rule := range hiringRules {
		if textMatches(sub, rule.phrases) {
			return rule.signal, 95
		}
		if textMatches(all, rule.phrases) {
			return rule.signal, 70
		}
	}
	return SignalNone, 0
}
