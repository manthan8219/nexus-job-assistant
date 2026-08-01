package resume

import "strings"

// titleBucket maps a set of profession keywords to suggested job titles.
// Used by the offline suggestion fallback so onboarding works for ANY
// profession without AI keys.
type titleBucket struct {
	keywords []string
	titles   []string
}

var titleCatalog = []titleBucket{
	{
		keywords: []string{"doctor", "physician", "cardiolog", "surgeon", "nurse",
			"medical", "health", "dentist", "pharma", "clinical", "radiology",
			"pediatric", "therapist", "psycholog", "vet"},
		titles: []string{"Cardiologist", "General Practitioner", "Registered Nurse",
			"Physician Assistant", "Medical Researcher"},
	},
	{
		keywords: []string{"software", "engineer", "backend", "frontend", "full-stack",
			"full stack", "developer", "programmer", "golang", "python", "java",
			"javascript", "typescript", "sre", "devops", "platform", "data engineer",
			"infra", "cloud", "api", "infrastructure"},
		titles: []string{"Backend Engineer", "Full-Stack Engineer",
			"Site Reliability Engineer", "Platform Engineer", "Software Engineer"},
	},
	{
		keywords: []string{"data scientist", "data science", "machine learning", "ml",
			"data analyst", "analytics", "statistician", "ai"},
		titles: []string{"Data Scientist", "Data Analyst",
			"Machine Learning Engineer", "Analytics Engineer"},
	},
	{
		keywords: []string{"design", "ux", "ui", "product designer", "graphic",
			"visual", "illustrator", "art"},
		titles: []string{"Product Designer", "UX/UI Designer",
			"Brand Designer", "Visual Designer"},
	},
	{
		keywords: []string{"marketing", "marketer", "growth", "seo", "content",
			"social", "brand", "demand", "campaign"},
		titles: []string{"Growth Marketing Manager", "Digital Marketing Specialist",
			"Content Marketing Manager", "SEO Specialist"},
	},
	{
		keywords: []string{"sales", "account executive", "account manager",
			"business development", "customer success", "sdr", "bdr"},
		titles: []string{"Account Executive",
			"Business Development Representative",
			"Customer Success Manager", "Sales Manager"},
	},
	{
		keywords: []string{"finance", "accountant", "accounting", "financial",
			"fp&a", "audit", "tax", "treasury", "investment", "banking"},
		titles: []string{"Financial Analyst", "Accountant", "FP&A Analyst",
			"Investment Banking Analyst"},
	},
	{
		keywords: []string{"teacher", "education", "professor", "instructor",
			"tutor", "lecturer", "curriculum", "academic"},
		titles: []string{"Teacher", "Curriculum Developer", "Professor",
			"Instructional Designer"},
	},
	{
		keywords: []string{"lawyer", "legal", "attorney", "paralegal",
			"counsel", "compliance"},
		titles: []string{"Corporate Lawyer", "Paralegal",
			"Compliance Officer", "Legal Counsel"},
	},
	{
		keywords: []string{"manager", "project manager", "product manager",
			"program manager", "operations", "ops", "coordinator", "director"},
		titles: []string{"Product Manager", "Project Manager",
			"Program Manager", "Operations Manager"},
	},
	{
		keywords: []string{"hr", "human resources", "recruiter", "talent",
			"people ops", "benefits"},
		titles: []string{"HR Business Partner", "Recruiter",
			"Talent Acquisition Specialist", "People Operations Manager"},
	},
	{
		keywords: []string{"writer", "writing", "editor", "journalist",
			"copywriter", "content writer", "author"},
		titles: []string{"Technical Writer", "Copywriter",
			"Content Editor", "Journalist"},
	},
}

// SuggestTitlesOffline returns profession-matched job title suggestions for an
// intent without calling any AI service. It is the no-keys fallback used by
// the API when AI Assist is off.
func SuggestTitlesOffline(intent string, _ string, _ []string) []string {
	text := strings.ToLower(strings.TrimSpace(intent))
	seen := make(map[string]bool)
	var out []string
	for _, bucket := range titleCatalog {
		matched := false
		for _, kw := range bucket.keywords {
			if strings.Contains(text, kw) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, t := range bucket.titles {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	if len(out) > 6 {
		out = out[:6]
	}
	if len(out) > 0 {
		return out
	}
	// Fallback: split the free-text intent on commas and drop noise tokens.
	if parts := splitIntentTokens(intent); len(parts) > 0 {
		if len(parts) > 6 {
			parts = parts[:6]
		}
		return parts
	}
	return []string{"Specialist", "Coordinator", "Analyst"}
}

// splitIntentTokens splits a free-text intent on commas, dropping salary and
// work-type noise tokens.
func splitIntentTokens(intent string) []string {
	var out []string
	for _, p := range strings.Split(intent, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		if strings.HasPrefix(low, "$") || low == "remote" || low == "onsite" ||
			low == "hybrid" || low == "remote-first" {
			continue
		}
		out = append(out, p)
	}
	return out
}
