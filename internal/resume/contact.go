package resume

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Contact holds the structured personal details extracted from a resume.
// It is used to backfill the user's profile so they only fill in what the
// resume does not contain.
type Contact struct {
	FirstName string   `json:"firstName,omitempty"`
	LastName  string   `json:"lastName,omitempty"`
	Email     string   `json:"email,omitempty"`
	Phone     string   `json:"phone,omitempty"`
	LinkedIn  string   `json:"linkedIn,omitempty"`
	Years     string   `json:"years,omitempty"`
	Skills    []string `json:"skills,omitempty"`
}

var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe    = regexp.MustCompile(`(\+?\d[\d\s().\-]{7,}\d)`)
	linkedInRe = regexp.MustCompile(`(?i)linkedin\.com/in/([A-Za-z0-9\-_]+)`)
	yearsRe    = regexp.MustCompile(`(?i)(\d{1,2})\s*\+?\s*years?`)
)

// cleanEmail trims PDF text-extraction artifacts that glue a following word to
// the TLD ("user@gmail.comGitHub"). TLDs are lowercase, so any uppercase text
// after the final dot is dropped.
func cleanEmail(raw string) string {
	idx := strings.LastIndex(raw, ".")
	if idx < 0 || idx == len(raw)-1 {
		return raw
	}
	for i, r := range raw[idx+1:] {
		if unicode.IsUpper(r) {
			return raw[:idx+1+i]
		}
	}
	return raw
}

// ExtractContact pulls name, email, phone, LinkedIn handle, and years of
// experience out of resume text using deterministic patterns (no AI needed).
func ExtractContact(text string, aiYears int) Contact {
	var c Contact
	if m := emailRe.FindString(text); m != "" {
		c.Email = strings.ToLower(cleanEmail(strings.TrimSpace(m)))
	}
	if m := phoneRe.FindString(text); m != "" {
		c.Phone = strings.TrimSpace(m)
	}
	if m := linkedInRe.FindStringSubmatch(text); len(m) > 1 {
		c.LinkedIn = strings.TrimSpace(m[1])
	}
	if aiYears > 0 {
		c.Years = strconv.Itoa(aiYears)
	} else if m := yearsRe.FindStringSubmatch(text); len(m) > 1 {
		c.Years = m[1]
	}
	c.FirstName, c.LastName = guessName(text)
	return c
}

// guessName looks for the resume's name in the first lines: a short line with
// two capitalized words that is not an email, URL, or phone number.
func guessName(text string) (first, last string) {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || len(line) > 48 {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "@") || strings.Contains(lower, "http") ||
			strings.Contains(lower, "linkedin") || strings.ContainsAny(line, "0123456789") {
			continue
		}
		words := strings.Fields(line)
		if len(words) < 2 || len(words) > 4 {
			continue
		}
		if !isCapitalized(words[0]) || !isCapitalized(words[1]) {
			continue
		}
		first = strings.TrimRight(words[0], ",.;")
		last = strings.Trim(strings.Join(words[1:], " "), " ,.;—–|-")
		return first, last
	}
	return "", ""
}

func isCapitalized(w string) bool {
	if w == "" {
		return false
	}
	return unicode.IsUpper([]rune(w)[0])
}

// enrichContact fills r.Contact from resume text, preferring the AI years
// estimate and skills when a profile is available.
func enrichContact(r *Result, text string, profile *Profile) {
	if strings.TrimSpace(text) == "" {
		return
	}
	years := 0
	if profile != nil {
		years = profile.YearsEstimate
	}
	c := ExtractContact(text, years)
	if profile != nil && len(c.Skills) == 0 && len(profile.Skills) > 0 {
		c.Skills = profile.Skills
	}
	r.Contact = c
}
