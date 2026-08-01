package resume

import (
	"regexp"
	"strings"
)

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

// professionBucket maps a friendly profession label to the keywords that
// detect it. Order matters: SuggestProfession returns the FIRST matching label,
// so specific domains (Data/AI, Research/Science) precede broad siblings
// (Engineering), and ambiguous roles (e.g. "HR manager") land on the domain
// they belong to rather than the generic Project Management bucket.
type professionBucket struct {
	label    string
	keywords []string
}

// professionCatalog drives SuggestProfession and mirrors the title buckets in
// titleCatalog while keeping the labels friendly and mutually exclusive.
var professionCatalog = []professionBucket{
	{
		label: "Healthcare",
		keywords: []string{"doctor", "physician", "cardiolog", "surgeon", "nurse",
			"medical", "health", "dentist", "pharma", "clinical", "radiology",
			"pediatric", "therapist", "psycholog", "veterinar", "vet"},
	},
	{
		label: "Data/AI",
		keywords: []string{"data scien", "data analyst", "data engineer",
			"machine learning", "deep learning", "artificial intelligence",
			"analytics", "statistician", "computer vision", "neural", "nlp",
			"llm", "mlops", "ml", "ai"},
	},
	{
		label: "Engineering",
		keywords: []string{"software", "engineer", "backend", "frontend",
			"full-stack", "full stack", "developer", "programmer", "golang",
			"python", "java", "javascript", "typescript", "sre", "devops",
			"platform", "infra", "infrastructure", "cloud", "apis", "api"},
	},
	{
		label: "Research/Science",
		keywords: []string{"research", "scientist", "biolog", "chemist",
			"chemistry", "physics", "genomics", "epidemiolog", "laboratory",
			"lab"},
	},
	{
		label: "Design",
		keywords: []string{"product designer", "design", "graphic", "visual",
			"illustrator", "artist", "ux", "ui", "art"},
	},
	{
		label: "Marketing",
		keywords: []string{"marketing", "marketer", "growth", "seo", "content",
			"social", "brand", "demand", "campaign"},
	},
	{
		label: "Sales",
		keywords: []string{"sales", "account executive", "account manager",
			"business development", "customer success", "sdr", "bdr"},
	},
	{
		label: "Finance",
		keywords: []string{"finance", "accountant", "accounting", "financial",
			"fp&a", "audit", "taxation", "taxes", "treasury", "investment",
			"banking", "tax"},
	},
	{
		label: "Education",
		keywords: []string{"teacher", "education", "professor", "instructor",
			"tutor", "lecturer", "curriculum", "academic"},
	},
	{
		label: "Legal",
		keywords: []string{"lawyer", "legal", "attorney", "paralegal", "counsel",
			"compliance"},
	},
	{
		label: "HR",
		keywords: []string{"human resources", "recruiter", "talent", "people ops",
			"benefits", "hr"},
	},
	{
		label: "Writing",
		keywords: []string{"writer", "writing", "editor", "journalist",
			"copywriter", "content writer", "author"},
	},
	{
		label: "Trade/Construction",
		keywords: []string{"electrician", "plumber", "plumbing", "carpenter",
			"carpentry", "welder", "hvac", "construction", "contractor",
			"machinist", "mechanic", "roofer"},
	},
	{
		label: "Customer Support",
		keywords: []string{"customer service", "helpdesk", "help desk",
			"call center", "technical support", "support"},
	},
	{
		label: "Project Management",
		keywords: []string{"project manager", "project management",
			"program manager", "product manager", "scrum", "agile", "pmo",
			"coordinator", "director", "operations", "manager"},
	},
}

// SuggestProfession keyword-detects the profession domain of a job intent and
// returns a friendly label ("Healthcare", "Engineering", "Data/AI", …), or ""
// when no domain is recognizable. It powers the profession-aware onboarding
// badge and mirrors the offline title catalog's keyword vocabulary.
func SuggestProfession(intent string) string {
	text := strings.ToLower(strings.TrimSpace(intent))
	if text == "" {
		return ""
	}
	for _, bucket := range professionCatalog {
		for _, kw := range bucket.keywords {
			if professionMatches(text, kw) {
				return bucket.label
			}
		}
	}
	return ""
}

var notWord = regexp.MustCompile(`[^a-z0-9]+`)

// professionMatches reports whether keyword appears in text. Longer phrases
// match as substrings; short tokens (≤ 3 chars, e.g. "ai", "hr", "ui") must
// match as whole words so they don't fire inside unrelated words ("email",
// "through", "built", "startup").
func professionMatches(text, keyword string) bool {
	if len(keyword) > 3 {
		return strings.Contains(text, keyword)
	}
	for _, tok := range notWord.Split(text, -1) {
		if tok == keyword {
			return true
		}
	}
	return false
}
