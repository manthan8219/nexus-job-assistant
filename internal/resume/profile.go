package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
)

// ScoredItem is a labeled 1–10 score for charts.
type ScoredItem struct {
	Name  string `json:"name"`
	Score int    `json:"score"` // 1-10
}

// Profile is an AI-generated career read of the resume.
type Profile struct {
	Summary         string       `json:"summary"`
	WhatsGood       []string     `json:"whatsGood"`
	WhatsWrong      []string     `json:"whatsWrong"`
	Strengths       []string     `json:"strengths"`
	StrengthScores  []ScoredItem `json:"strengthScores"`
	SuitableRoles   []string     `json:"suitableRoles"`
	RoleFit         []ScoredItem `json:"roleFit"`
	Skills          []string     `json:"skills"`
	SkillScores     []ScoredItem `json:"skillScores"`
	ExperienceLevel string       `json:"experienceLevel"`
	YearsEstimate   int          `json:"yearsEstimate"`
	Industries      []string     `json:"industries"`
	Improvements    []string     `json:"improvements"`
	Error           string       `json:"-"`
}

// AIOptions controls whether/how AI profile analysis runs.
type AIOptions struct {
	Enabled      bool
	Provider     string // "local" | "api"
	LocalURL     string
	LocalModel   string
	OpenAIKey    string
	AnthropicKey string
}

// AIOptionsFromConfig builds AIOptions from the persisted user config so
// callers outside the engine (e.g. outreach UI) don't duplicate the mapping.
func AIOptionsFromConfig(cfg *config.Config) AIOptions {
	if cfg == nil {
		return AIOptions{}
	}
	return AIOptions{
		Enabled:      cfg.AIAssist,
		Provider:     cfg.AIProvider,
		LocalURL:     cfg.LocalLLMURL,
		LocalModel:   cfg.LocalLLMModel,
		OpenAIKey:    cfg.OpenAIKey,
		AnthropicKey: cfg.AnthropicKey,
	}
}

// AnalyzeFull validates the file, then optionally builds an AI profile.
func AnalyzeFull(path string, ai AIOptions) Result {
	r := Analyze(path)
	if !r.Valid || !ai.Enabled {
		return r
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	text, err := ExtractText(path)
	if err != nil {
		r.Profile = &Profile{Error: err.Error()}
		r.Message += " · AI pending (need readable text)"
		return r
	}

	profile, err := buildProfile(ctx, text, ai)
	if err != nil {
		r.Profile = &Profile{Error: err.Error()}
		r.Message += " · AI analysis failed"
		return r
	}
	r.Profile = profile
	r.Message += " · AI profile ready"
	return r
}

func buildProfile(ctx context.Context, resumeText string, ai AIOptions) (*Profile, error) {
	prompt := profilePrompt(resumeText)
	raw, err := complete(ctx, ai, prompt)
	if err != nil {
		return nil, err
	}
	p, err := parseProfile(raw)
	if err != nil {
		return nil, err
	}
	normalizeScores(p)
	return p, nil
}

func profilePrompt(resumeText string) string {
	return `You are a blunt but fair technical recruiter and career coach.

TASK: Critically review this resume. Give a BALANCED judgment — equal weight to what is strong and what is weak. Do not flatter. Do not only criticize.

BALANCE REQUIREMENT:
- whatsGood and whatsWrong must each have 4–6 items of similar specificity and seriousness.
- If you praise something, the critique section must still find real gaps (scope, evidence, seniority mismatch, missing metrics, narrow stack, unclear impact, etc.).
- If you criticize something, acknowledge genuine strengths with equal concreteness.
- summary must include BOTH a clear positive framing AND an honest caveat (at least one sentence each).

RULES:
- Use ONLY facts grounded in the resume. Never invent employers, degrees, companies, or skills.
- Critique the candidate's positioning AND the resume's quality (clarity, proof, focus).
- Prefer uncomfortable truths recruiters would actually say in a debrief.
- Ignore PDF artifacts, icon glyphs, broken spacing, formatting noise; fix split words when obvious.
- Respond with ONE JSON object only. No markdown, no code fences, no commentary.

JSON SCHEMA (all keys required):
{
  "summary": "4-6 sentences: balanced overview — who they are, what they are good at, and what holds them back",
  "whatsGood": ["4-6 specific positives about the candidate or resume, each with implied evidence"],
  "whatsWrong": ["4-6 specific weaknesses, risks, or missing proof a hiring manager would notice"],
  "strengths": ["4-6 concrete strengths tied to resume evidence"],
  "strengthScores": [{"name":"short strength label","score":1-10}, ... 4-6 items],
  "suitableRoles": ["5-8 realistic job titles — not stretch titles they cannot defend"],
  "roleFit": [{"name":"job title","score":1-10 fit}, ... 4-6 items],
  "skills": ["8-14 hard skills / tools explicitly present"],
  "skillScores": [{"name":"skill","score":1-10 proficiency inferred}, ... 5-8 top skills],
  "experienceLevel": "intern|junior|mid|senior|lead|executive",
  "yearsEstimate": 0,
  "industries": ["3-6 industries that fit"],
  "improvements": ["3-5 actionable fixes that address the whatsWrong items"]
}

SCORING GUIDE: 1=weak/absent, 5=solid, 8=strong, 10=exceptional for their level.
Be calibrated — do not score everything 8–9.

RESUME TEXT:
"""
` + resumeText + `
"""`
}

// Complete routes a raw prompt to whichever AI backend AIOptions selects
// (local Ollama or a cloud API key) and returns the raw completion text.
// Exported for callers outside this package that need free-form AI
// generation without the resume-specific prompt templates (e.g.
// application question answering).
func Complete(ctx context.Context, ai AIOptions, prompt string) (string, error) {
	return complete(ctx, ai, prompt)
}

func complete(ctx context.Context, ai AIOptions, prompt string) (string, error) {
	switch strings.ToLower(ai.Provider) {
	case "api":
		if ai.AnthropicKey != "" {
			return completeAnthropic(ctx, ai.AnthropicKey, prompt)
		}
		if ai.OpenAIKey != "" {
			return completeOpenAI(ctx, ai.OpenAIKey, prompt)
		}
		return "", fmt.Errorf("AI backend is API Keys but no Anthropic/OpenAI key is set")
	default:
		client := localllm.NewClient(ai.LocalURL)
		if err := client.Ping(ctx); err != nil {
			return "", err
		}
		return client.GenerateJSON(ctx, ai.LocalModel, prompt)
	}
}

func parseProfile(raw string) (*Profile, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var p Profile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("model returned invalid JSON: %w", err)
	}
	if p.Summary == "" && len(p.Strengths) == 0 {
		return nil, fmt.Errorf("empty AI profile")
	}
	sanitizeProfile(&p)
	return &p, nil
}

// sanitizeProfile strips common LLM hallucinated references (image, chart, figure)
// from all text fields of the profile so the UI never shows broken placeholders.
func sanitizeProfile(p *Profile) {
	p.Summary = sanitizeAIOutput(p.Summary)
	p.WhatsGood = sanitizeSlice(p.WhatsGood)
	p.WhatsWrong = sanitizeSlice(p.WhatsWrong)
	p.Strengths = sanitizeSlice(p.Strengths)
	p.SuitableRoles = sanitizeSlice(p.SuitableRoles)
	p.Skills = sanitizeSlice(p.Skills)
	p.Industries = sanitizeSlice(p.Industries)
	p.Improvements = sanitizeSlice(p.Improvements)
}

func sanitizeSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = sanitizeAIOutput(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// sanitizeAIOutput removes markdown image references like [Image 1], [Chart 2],
// [Figure 3], and similar LLM hallucinated placeholders from text.
func sanitizeAIOutput(s string) string {
	// Remove [Image N], [Chart N], [Figure N], [Table N], [Graph N] references.
	re := regexp.MustCompile(`\[(?:Image|Chart|Figure|Table|Graph|Diagram|Picture|Screenshot|Screen shot)\s*\d*\]`)
	s = re.ReplaceAllString(s, "")
	// Remove any dangling brackets like "[]" left over.
	s = strings.ReplaceAll(s, "[]", "")
	// Clean up double spaces.
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func normalizeScores(p *Profile) {
	clamp := func(items []ScoredItem) []ScoredItem {
		out := make([]ScoredItem, 0, len(items))
		for _, it := range items {
			it.Name = strings.TrimSpace(it.Name)
			if it.Name == "" {
				continue
			}
			if it.Score < 1 {
				it.Score = 1
			}
			if it.Score > 10 {
				it.Score = 10
			}
			out = append(out, it)
		}
		return out
	}
	// Ensure every list field is a non-nil slice so JSON emits [] not null
	// (the UI relies on .map/.slice and would crash on null).
	clean := func(items []string) []string {
		out := make([]string, 0, len(items))
		for _, s := range items {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	p.WhatsGood = clean(p.WhatsGood)
	p.WhatsWrong = clean(p.WhatsWrong)
	p.Strengths = clean(p.Strengths)
	p.SuitableRoles = clean(p.SuitableRoles)
	p.Skills = clean(p.Skills)
	p.Industries = clean(p.Industries)
	p.Improvements = clean(p.Improvements)
	p.StrengthScores = clamp(p.StrengthScores)
	p.RoleFit = clamp(p.RoleFit)
	p.SkillScores = clamp(p.SkillScores)

	// Synthesize bars from lists if the model omitted scores.
	if len(p.StrengthScores) == 0 {
		for i, s := range p.Strengths {
			if i >= 6 {
				break
			}
			p.StrengthScores = append(p.StrengthScores, ScoredItem{Name: truncateLabel(s, 28), Score: 8 - i})
		}
	}
	if len(p.RoleFit) == 0 {
		for i, s := range p.SuitableRoles {
			if i >= 6 {
				break
			}
			p.RoleFit = append(p.RoleFit, ScoredItem{Name: truncateLabel(s, 28), Score: 9 - i})
		}
	}
	if len(p.SkillScores) == 0 {
		for i, s := range p.Skills {
			if i >= 8 {
				break
			}
			p.SkillScores = append(p.SkillScores, ScoredItem{Name: truncateLabel(s, 22), Score: 8 - (i / 2)})
		}
	}
}

func truncateLabel(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
