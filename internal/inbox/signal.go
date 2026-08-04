package inbox

import "strings"

// hiringRule pairs a signal with the phrases that indicate it. The classifier
// walks rules in order - the first match wins, so ordering encodes priority
// (rejection before offer before interview, etc.). Phrases are kept hiring-
// specific to avoid labelling marketing/promo mail as a hiring signal.
type hiringRule struct {
	signal  Signal
	phrases []string
}

// rejectionKeywords are specific enough to trust anywhere (subject or body),
// so a rejection always wins over a coincidental interview mention.
var rejectionKeywords = []string{
	"unfortunately", "regret to inform", "not moving forward", "not been selected",
	"not selected", "other candidates", "position has been filled", "decided not to proceed",
	"unable to move forward", "no longer be considered", "will not be moving forward",
	"not proceeding", "after careful consideration",
}

var hiringRules = []hiringRule{
	{SignalRejection, rejectionKeywords},
	{SignalInterview, []string{
		"interview", "phone screen", "technical screen", "screening call", "assessment day",
		"onsite interview", "virtual interview", "schedule an interview",
		"would love to schedule", "we'd love to schedule",
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
		"recruiter", "found your profile", "came across your profile", "matched your profile",
		"headhunter", "talent acquisition", "we're hiring", "we are hiring", "opportunity at",
		"about your background", "impressed by your background", "would be a great fit",
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
// Rejection is checked against the whole message first (it must win over a
// coincidental interview mention). Other signals prefer a subject match; a
// body-only match is lower confidence, and a bare body "interview" is too
// noisy (promo/tutoring mail and job digests) to trust without the subject.
func Classify(subject, body string) (Signal, int) {
	sub := strings.ToLower(subject)
	all := strings.ToLower(body)
	if textMatches(sub+" "+all, rejectionKeywords) {
		return SignalRejection, 95
	}
	for _, rule := range hiringRules {
		if rule.signal == SignalRejection {
			continue
		}
		if textMatches(sub, rule.phrases) {
			return rule.signal, 95
		}
	}
	for _, rule := range hiringRules {
		if rule.signal == SignalRejection {
			continue
		}
		if rule.signal == SignalInterview && textMatches(all, []string{"interview"}) {
			continue
		}
		if textMatches(all, rule.phrases) {
			return rule.signal, 70
		}
	}
	return SignalNone, 0
}
