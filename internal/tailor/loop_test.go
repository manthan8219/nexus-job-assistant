package tailor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// scripted fakes implementing the three agent interfaces.

type fakeCVWriter struct {
	err   error
	calls []WriterInput
}

func (f *fakeCVWriter) Run(_ context.Context, in WriterInput) (resume.ImprovedDoc, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return resume.ImprovedDoc{}, f.err
	}
	return resume.ImprovedDoc{
		FullName: "Ada Lovelace",
		Summary:  "Backend engineer.",
		Skills:   []string{"Go"},
		Experience: []resume.ImprovedRole{
			{Title: "Backend Engineer", Org: "Example", Period: "2023 – Present", Bullets: []string{"Built APIs"}},
		},
	}, nil
}

type fakeCoverWriter struct {
	err   error
	calls []WriterInput
}

func (f *fakeCoverWriter) Run(_ context.Context, in WriterInput) (resume.CoverLetter, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return resume.CoverLetter{}, f.err
	}
	return resume.CoverLetter{
		Greeting:   "Dear Acme Hiring Team,",
		Paragraphs: []string{"I am a strong match.", "My Go work maps to your needs.", "I would love to talk."},
		Closing:    "Sincerely,",
		Signature:  "Ada Lovelace",
	}, nil
}

type fakeHR struct {
	byRound map[int]HRReview
	err     error
	calls   []ReviewInput
}

func (f *fakeHR) Run(_ context.Context, in ReviewInput) (HRReview, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return HRReview{}, f.err
	}
	if rev, ok := f.byRound[in.Round]; ok {
		return rev, nil
	}
	return HRReview{Verdict: "pass", ATSScore: 90, HRScore: 90, Summary: "great"}, nil
}

func testInput(t *testing.T, rounds int) Input {
	t.Helper()
	return Input{
		Job: provider.Job{
			Title: "Backend Engineer", Company: "Acme",
			Description: "We need a Go engineer with gRPC experience.",
		},
		ResumeText: "Ada Lovelace — backend engineer. Go, PostgreSQL, AWS. Built APIs at Example Corp.",
		MaxRounds:  rounds,
		OutDir:     filepath.Join(t.TempDir(), "kit"),
	}
}

func assertKitFiles(t *testing.T, out *Output) {
	t.Helper()
	for _, p := range []string{out.ResumePDF, out.ResumeTeX, out.ResumeMD, out.ResumeJSON,
		out.CoverPDF, out.CoverTeX, out.CoverMD, out.ReviewJSON} {
		if p == "" {
			t.Fatalf("output path empty: %+v", out)
		}
		info, err := os.Stat(p)
		if err != nil || info.Size() == 0 {
			t.Errorf("kit file %s missing/empty (err=%v)", p, err)
		}
	}
}

func TestGeneratePassesRound1(t *testing.T) {
	hr := &fakeHR{byRound: map[int]HRReview{
		1: {Verdict: "pass", ATSScore: 88, HRScore: 81, Summary: "strong"},
	}}
	out, err := generate(context.Background(), agents{cv: &fakeCVWriter{}, cover: &fakeCoverWriter{}, hr: hr}, testInput(t, 3))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !out.Passed || out.Rounds != 1 {
		t.Errorf("Passed=%v Rounds=%d; want true/1", out.Passed, out.Rounds)
	}
	if out.Review.ATSScore != 88 {
		t.Errorf("final review ATS=%d; want 88", out.Review.ATSScore)
	}
	assertKitFiles(t, out)
}

func TestGenerateRevisesWithFeedbackThenPasses(t *testing.T) {
	cv := &fakeCVWriter{}
	hr := &fakeHR{byRound: map[int]HRReview{
		1: {Verdict: "revise", ATSScore: 50, HRScore: 55, Summary: "weak",
			MissingKeywords: []string{"grpc"}, Issues: []string{"add grpc evidence"}, Feedback: "weave in grpc"},
		2: {Verdict: "pass", ATSScore: 85, HRScore: 80, Summary: "better"},
	}}
	out, err := generate(context.Background(), agents{cv: cv, cover: &fakeCoverWriter{}, hr: hr}, testInput(t, 3))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !out.Passed || out.Rounds != 2 {
		t.Fatalf("Passed=%v Rounds=%d; want true/2", out.Passed, out.Rounds)
	}
	if len(out.History) != 2 {
		t.Fatalf("history len %d; want 2", len(out.History))
	}
	// Round 2 must receive round 1's HR feedback verbatim.
	if len(cv.calls) != 2 {
		t.Fatalf("cv writer called %d times; want 2", len(cv.calls))
	}
	if cv.calls[0].Feedback != "" {
		t.Errorf("round 1 feedback = %q; want empty", cv.calls[0].Feedback)
	}
	fb := cv.calls[1].Feedback
	for _, want := range []string{"grpc", "add grpc evidence", "50", "55"} {
		if !strings.Contains(fb, want) {
			t.Errorf("round 2 feedback missing %q:\n%s", want, fb)
		}
	}
}

func TestGenerateNeverPassesKeepsBestRound(t *testing.T) {
	hr := &fakeHR{byRound: map[int]HRReview{
		1: {Verdict: "revise", ATSScore: 40, HRScore: 45, Summary: "r1", Issues: []string{"x"}},
		2: {Verdict: "revise", ATSScore: 60, HRScore: 65, Summary: "r2 better", Issues: []string{"y"}},
	}}
	out, err := generate(context.Background(), agents{cv: &fakeCVWriter{}, cover: &fakeCoverWriter{}, hr: hr}, testInput(t, 2))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if out.Passed {
		t.Fatal("Passed=true; want false when HR never approves")
	}
	if out.Rounds != 2 || out.Review.Summary != "r2 better" {
		t.Errorf("Rounds=%d final=%q; want 2/'r2 better' (best-scoring round)", out.Rounds, out.Review.Summary)
	}
	assertKitFiles(t, out)
}

func TestGenerateWriterErrorWrapsRound(t *testing.T) {
	ag := agents{
		cv:    &fakeCVWriter{err: errors.New("model down")},
		cover: &fakeCoverWriter{},
		hr:    &fakeHR{},
	}
	_, err := generate(context.Background(), ag, testInput(t, 3))
	if err == nil || !strings.Contains(err.Error(), "round 1 cv writer") {
		t.Fatalf("error = %v; want round-scoped cv writer error", err)
	}
}

func TestGenerateValidation(t *testing.T) {
	ag := agents{cv: &fakeCVWriter{}, cover: &fakeCoverWriter{}, hr: &fakeHR{}}

	in := testInput(t, 1)
	in.Job.Title = ""
	if _, err := generate(context.Background(), ag, in); err == nil {
		t.Fatal("missing title: expected error")
	}

	in = testInput(t, 1)
	in.Job.Company = ""
	if _, err := generate(context.Background(), ag, in); err == nil {
		t.Fatal("missing company: expected error")
	}

	in = testInput(t, 1)
	in.ResumeText = ""
	if _, err := generate(context.Background(), ag, in); err == nil {
		t.Fatal("missing resume text: expected error")
	}
}
