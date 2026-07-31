package tailor

import (
	"strings"
	"testing"
)

func TestParseHRReview(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantErr     bool
		wantVerdict string
		wantATS     int
		wantHR      int
	}{
		{
			name: "clean pass",
			raw: `{"verdict":"pass","ats_score":88,"hr_score":81,"ats_ready":true,
			       "would_interview":true,"missing_keywords":[],"issues":[],
			       "feedback":"ship it","summary":"strong match"}`,
			wantVerdict: "pass", wantATS: 88, wantHR: 81,
		},
		{
			name: "fenced revise",
			raw: "```json\n{\"verdict\":\"revise\",\"ats_score\":52,\"hr_score\":61," +
				"\"issues\":[\"missing go keyword\"],\"feedback\":\"add go\",\"summary\":\"close\"}\n```",
			wantVerdict: "revise", wantATS: 52, wantHR: 61,
		},
		{
			name:        "verdict derived from high scores",
			raw:         `{"ats_score":90,"hr_score":80,"summary":"great"}`,
			wantVerdict: "pass", wantATS: 90, wantHR: 80,
		},
		{
			name:        "verdict derived from low scores",
			raw:         `{"ats_score":40,"hr_score":80,"summary":"weak ats"}`,
			wantVerdict: "revise", wantATS: 40, wantHR: 80,
		},
		{
			name:        "scores clamped",
			raw:         `{"verdict":"pass","ats_score":140,"hr_score":-5,"summary":"x"}`,
			wantVerdict: "pass", wantATS: 100, wantHR: 0,
		},
		{name: "no json", raw: "the cv is fine", wantErr: true},
		{name: "empty review", raw: `{"ats_score":50,"hr_score":50}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHRReview(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseHRReview(%q) expected error, got %+v", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHRReview(%q) error: %v", tt.raw, err)
			}
			if got.Verdict != tt.wantVerdict || got.ATSScore != tt.wantATS || got.HRScore != tt.wantHR {
				t.Errorf("parseHRReview = verdict %q ats %d hr %d; want %q %d %d",
					got.Verdict, got.ATSScore, got.HRScore, tt.wantVerdict, tt.wantATS, tt.wantHR)
			}
		})
	}
}

func TestHRReviewPass(t *testing.T) {
	tests := []struct {
		name string
		rev  HRReview
		want bool
	}{
		{"explicit pass beats low scores", HRReview{Verdict: "pass", ATSScore: 10, HRScore: 10}, true},
		{"threshold pass despite revise verdict", HRReview{Verdict: "revise", ATSScore: 75, HRScore: 70}, true},
		{"below ats threshold", HRReview{Verdict: "revise", ATSScore: 74, HRScore: 90}, false},
		{"below hr threshold", HRReview{Verdict: "revise", ATSScore: 90, HRScore: 69}, false},
		{"case-insensitive verdict", HRReview{Verdict: " PASS "}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rev.Pass(); got != tt.want {
				t.Errorf("Pass() = %v; want %v (review %+v)", got, tt.want, tt.rev)
			}
		})
	}
}

func TestFeedbackBlock(t *testing.T) {
	rev := HRReview{
		ATSScore: 52, HRScore: 61,
		MissingKeywords: []string{"go", "grpc"},
		Issues:          []string{"no metrics in bullets", "summary too generic"},
		Feedback:        "quantify impact",
	}
	block := rev.feedbackBlock()
	for _, want := range []string{"52", "61", "go, grpc", "1. no metrics in bullets", "2. summary too generic", "quantify impact"} {
		if !strings.Contains(block, want) {
			t.Errorf("feedbackBlock missing %q:\n%s", want, block)
		}
	}
}
