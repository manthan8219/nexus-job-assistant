package greenhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testForm builds a small but representative apply form: basic fields, a
// required resume, a LinkedIn text question, a boolean yes/no question, and
// an option-ID select.
func testForm(submitPath string) *FormInfo {
	return &FormInfo{
		Board:       "testco",
		JobID:       "123",
		Title:       "Backend Engineer",
		Company:     "TestCo",
		SubmitPath:  submitPath,
		Fingerprint: "fp-abc",
		Questions: []ghQuestion{
			{Required: true, Label: "First Name", Fields: []ghField{{Name: "first_name", Type: "input_text"}}},
			{Required: true, Label: "Last Name", Fields: []ghField{{Name: "last_name", Type: "input_text"}}},
			{Required: true, Label: "Email", Fields: []ghField{{Name: "email", Type: "input_text"}}},
			{Required: true, Label: "Resume/CV", Fields: []ghField{{Name: "resume", Type: "input_file"}}},
			{Required: false, Label: "LinkedIn Profile", Fields: []ghField{{Name: "question_101", Type: "input_text"}}},
			{Required: true, Label: "Are you legally authorized to work here?", Fields: []ghField{{
				Name: "question_102", Type: "multi_value_single_select",
				Values: []ghValue{{Value: json.RawMessage(`1`), Label: "Yes"}, {Value: json.RawMessage(`0`), Label: "No"}},
			}}},
			{Required: true, Label: "How did you hear about this job?", Fields: []ghField{{
				Name: "question_103", Type: "multi_value_single_select",
				Values: []ghValue{{Value: json.RawMessage(`501`), Label: "LinkedIn"}, {Value: json.RawMessage(`502`), Label: "Referral"}},
			}}},
			{Required: true, Label: "Please review the privacy notice", Fields: []ghField{{
				Name: "question_104[]", Type: "multi_value_multi_select",
				Values: []ghValue{{Value: json.RawMessage(`601`), Label: "I acknowledge"}},
			}}},
		},
	}
}

func TestFetchForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/embed/job_app") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("_data"); got != "routes/embed.job_app" {
			t.Errorf("loader _data = %q, want routes/embed.job_app", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"submitPath": "https://boards.example/embed/testco/jobs/123",
			"confirmationPath": "/embed/job_app/confirmation",
			"urlToken": "testco",
			"jobPostId": 123,
			"jobPost": {
				"title": "Backend Engineer",
				"company_name": "TestCo",
				"job_post_location": "Remote",
				"fingerprint": "fp-abc",
				"questions": [
					{"required": true, "label": "First Name", "fields": [{"name": "first_name", "type": "input_text"}]}
				]
			}
		}`)
	}))
	defer srv.Close()

	old := boardsBaseURL
	boardsBaseURL = srv.URL
	defer func() { boardsBaseURL = old }()

	form, err := FetchForm(context.Background(), srv.Client(), "testco", "123")
	if err != nil {
		t.Fatalf("FetchForm: %v", err)
	}
	if form.SubmitPath != "https://boards.example/embed/testco/jobs/123" {
		t.Errorf("SubmitPath = %q", form.SubmitPath)
	}
	if form.Fingerprint != "fp-abc" {
		t.Errorf("Fingerprint = %q", form.Fingerprint)
	}
	if form.Title != "Backend Engineer" || form.Company != "TestCo" {
		t.Errorf("Title/Company = %q/%q", form.Title, form.Company)
	}
	if len(form.Questions) != 1 {
		t.Fatalf("Questions = %d, want 1", len(form.Questions))
	}
}

func TestFetchForm_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	oldBoards, oldJobBoards := boardsBaseURL, jobBoardsBaseURL
	boardsBaseURL, jobBoardsBaseURL = srv.URL, srv.URL
	defer func() { boardsBaseURL, jobBoardsBaseURL = oldBoards, oldJobBoards }()

	if _, err := FetchForm(context.Background(), srv.Client(), "nope", "1"); err == nil {
		t.Fatal("expected error for missing board, got nil")
	}
}

func TestQuestionID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"question_101", 101, true},
		{"question_67517575[]", 67517575, true},
		{"first_name", 0, false},
		{"question_abc", 0, false},
	}
	for _, c := range cases {
		got, ok := questionID(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("questionID(%q) = %d,%v; want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestMatchOption(t *testing.T) {
	values := []ghValue{
		{Value: json.RawMessage(`501`), Label: "LinkedIn"},
		{Value: json.RawMessage(`502`), Label: "Employee Referral"},
	}
	cases := []struct {
		want    string
		wantVal string
		wantOK  bool
	}{
		{"linkedin", "501", true},          // case-insensitive exact
		{"Employee Referral", "502", true}, // exact
		{"referral", "502", true},          // substring fallback
		{"twitter", "", false},
	}
	for _, c := range cases {
		v, ok := matchOption(c.want, values)
		if ok != c.wantOK || (ok && v.ValueStr() != c.wantVal) {
			t.Errorf("matchOption(%q) = %v,%v; want value=%q ok=%v", c.want, v.ValueStr(), ok, c.wantVal, c.wantOK)
		}
	}
}

func testProfile() provider.Profile {
	return provider.Profile{
		FirstName:  "Ada",
		LastName:   "Lovelace",
		Email:      "ada@example.com",
		Phone:      "+1 555 0100",
		LinkedInID: "adalovelace",
		City:       "Pune",
		ResumePath: "resume.pdf", // placeholder; upload tests override with a real temp file
	}
}

func TestBuildApplication(t *testing.T) {
	form := testForm("http://unused")
	profile := testProfile()
	byField := map[string]string{
		"question_101":   "https://linkedin.com/in/adalovelace",
		"question_102":   "Yes",
		"question_103":   "LinkedIn",
		"question_104[]": "I acknowledge",
	}

	app, missing, err := buildApplication(form, profile, byField, SubmitOptions{})
	if err != nil {
		t.Fatalf("buildApplication: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}

	if app["first_name"] != "Ada" || app["last_name"] != "Lovelace" || app["email"] != "ada@example.com" {
		t.Errorf("basic fields wrong: %v %v %v", app["first_name"], app["last_name"], app["email"])
	}
	if app["phone"] != "+1 555 0100" {
		t.Errorf("phone = %v", app["phone"])
	}
	if app["from_job_board_renderer"] != true {
		t.Error("from_job_board_renderer must be true")
	}

	answers := app["answers_attributes"].(map[string]any)

	text := answers["101"].(map[string]any)
	if text["text_value"] != "https://linkedin.com/in/adalovelace" {
		t.Errorf("q101 text_value = %v", text["text_value"])
	}
	if text["question_id"].(int64) != 101 {
		t.Errorf("q101 question_id = %v", text["question_id"])
	}

	boolean := answers["102"].(map[string]any)
	if boolean["boolean_value"] != 1 {
		t.Errorf("q102 boolean_value = %v, want 1", boolean["boolean_value"])
	}
	if _, hasOptions := boolean["answer_selected_options_attributes"]; hasOptions {
		t.Error("q102 must use boolean_value, not option attributes")
	}

	sel := answers["103"].(map[string]any)
	opts := sel["answer_selected_options_attributes"].(map[string]any)
	first := opts["0"].(map[string]any)
	if first["question_option_id"].(int64) != 501 {
		t.Errorf("q103 option id = %v, want 501", first["question_option_id"])
	}

	cb := answers["104"].(map[string]any)
	cbOpts := cb["answer_selected_options_attributes"].(map[string]any)
	if cbOpts["0"].(map[string]any)["question_option_id"].(int64) != 601 {
		t.Errorf("q104 option id = %v, want 601", cbOpts["0"])
	}

	// Priorities are assigned in question order across custom questions.
	for i, key := range []string{"101", "102", "103", "104"} {
		if p := answers[key].(map[string]any)["priority"].(int); p != i {
			t.Errorf("q%s priority = %d, want %d", key, p, i)
		}
	}
}

func TestBuildApplication_MissingRequired(t *testing.T) {
	form := testForm("http://unused")
	// Leave the required yes/no and select questions unanswered.
	byField := map[string]string{"question_101": "x"}

	_, missing, err := buildApplication(form, testProfile(), byField, SubmitOptions{})
	if err != nil {
		t.Fatalf("buildApplication: %v", err)
	}
	if len(missing) != 3 { // work auth + hear-about + privacy checkbox
		t.Fatalf("missing = %v, want 3 entries", missing)
	}
}

// mockGreenhouse serves the presigned-fields endpoint, the S3 upload, and the
// application submit endpoint, capturing the submitted JSON body.
type mockGreenhouse struct {
	*httptest.Server
	submitStatus int
	lastBody     map[string]any
	s3Hits       int
}

func newMockGreenhouse(t *testing.T, submitStatus int) *mockGreenhouse {
	m := &mockGreenhouse{submitStatus: submitStatus}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/uncacheable_attributes/presigned_fields"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"url": "%s/s3", "resume": {"fields": {"policy": "p", "x-amz-signature": "s"}, "key": "stash/{timestamp}-{unique_id}-abc"}}`, m.URL)
		case r.URL.Path == "/s3":
			m.s3Hits++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `<?xml version="1.0"?><PostResponse><Location>`+m.URL+`/stash/resume.pdf</Location><Key>stash/resume.pdf</Key></PostResponse>`)
		case r.URL.Path == "/submit":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &m.lastBody); err != nil {
				t.Errorf("submit body is not valid JSON: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(m.submitStatus)
			if m.submitStatus == 422 {
				fmt.Fprint(w, `{"code":"unprocessable-entity","message":"Email is invalid"}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	return m
}

func TestSubmitApplication(t *testing.T) {
	resumePath := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(resumePath, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name         string
		submitStatus int
		wantStatus   string
	}{
		{"success", 200, "applied"},
		{"created", 201, "applied"},
		{"captcha missing token", 400, "skipped"},
		{"captcha failed", 428, "skipped"},
		{"validation error", 422, "failed"},
		{"server error", 500, "failed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newMockGreenhouse(t, c.submitStatus)
			defer mock.Close()

			old := boardsBaseURL
			boardsBaseURL = mock.URL
			defer func() { boardsBaseURL = old }()

			form := testForm(mock.URL + "/submit")
			profile := testProfile()
			profile.ResumePath = resumePath

			answers := []Answer{
				{Question: form.Questions[4], Value: "https://linkedin.com/in/adalovelace"},
				{Question: form.Questions[5], Value: "Yes"},
				{Question: form.Questions[6], Value: "LinkedIn"},
				{Question: form.Questions[7], Value: "I acknowledge"},
			}

			res, err := submitApplication(context.Background(), mock.Client(), form, profile, answers, SubmitOptions{})
			if err != nil {
				t.Fatalf("submitApplication: %v", err)
			}
			if res.Status != c.wantStatus {
				t.Fatalf("status = %q, want %q (reason: %s)", res.Status, c.wantStatus, res.Reason)
			}

			if mock.s3Hits != 1 {
				t.Errorf("s3 uploads = %d, want 1 (resume)", mock.s3Hits)
			}

			app := mock.lastBody["job_application"].(map[string]any)
			if app["email"] != "ada@example.com" {
				t.Errorf("submitted email = %v", app["email"])
			}
			if app["resume_url"] != mock.URL+"/stash/resume.pdf" {
				t.Errorf("resume_url = %v", app["resume_url"])
			}
			if app["resume_url_filename"] != "resume.pdf" {
				t.Errorf("resume_url_filename = %v", app["resume_url_filename"])
			}
			if mock.lastBody["fingerprint"] != "fp-abc" {
				t.Errorf("fingerprint = %v", mock.lastBody["fingerprint"])
			}
		})
	}
}

func TestSubmitApplication_SkipsBeforeUploadWhenRequiredMissing(t *testing.T) {
	mock := newMockGreenhouse(t, 200)
	defer mock.Close()

	old := boardsBaseURL
	boardsBaseURL = mock.URL
	defer func() { boardsBaseURL = old }()

	form := testForm(mock.URL + "/submit")
	profile := testProfile()
	// nil answers → required custom questions stay unanswered → must skip.

	res, err := submitApplication(context.Background(), mock.Client(), form, profile, nil, SubmitOptions{})
	if err != nil {
		t.Fatalf("submitApplication: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
	if mock.s3Hits != 0 {
		t.Errorf("s3 uploads = %d, want 0 — nothing uploaded when validation fails", mock.s3Hits)
	}
	if mock.lastBody != nil {
		t.Error("submit endpoint must not be hit when required fields are missing")
	}
}

func TestAutoAnswers(t *testing.T) {
	form := testForm("http://unused")
	answers := AutoAnswers(form.Questions, testProfile())
	if len(answers) != 4 {
		t.Fatalf("AutoAnswers returned %d, want 4 custom questions", len(answers))
	}

	byQID := map[int64]string{}
	for _, a := range answers {
		id, _ := questionID(a.Question.Fields[0].Name)
		byQID[id] = a.Value
	}

	if got := byQID[101]; !strings.Contains(got, "linkedin.com/in/adalovelace") {
		t.Errorf("LinkedIn answer = %q", got)
	}
	if got := byQID[102]; got != "Yes" {
		t.Errorf("work auth answer = %q, want Yes", got)
	}
	if got := byQID[103]; got != "LinkedIn" {
		t.Errorf("hear-about answer = %q, want LinkedIn", got)
	}
	if got := byQID[104]; got != "I acknowledge" {
		t.Errorf("consent checkbox answer = %q, want I acknowledge", got)
	}
}
