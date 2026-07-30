package greenhouse

import "encoding/json"

// API response types for Greenhouse Job Board API v1
// https://boards-api.greenhouse.io/v1/boards/{board}/jobs

type jobsResponse struct {
	Jobs []ghJob `json:"jobs"`
}

type ghJob struct {
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	UpdatedAt   string       `json:"updated_at"`
	Location    ghLocation   `json:"location"`
	Content     string       `json:"content"` // HTML job description when ?content=true
	Questions   []ghQuestion `json:"questions"` // populated when ?content=true
}

type ghLocation struct {
	Name string `json:"name"`
}

type ghQuestion struct {
	Required    bool      `json:"required"`
	Label       string    `json:"label"`
	Fields      []ghField `json:"fields"`
	Description string    `json:"description"`
}

type ghField struct {
	Name   string    `json:"name"`   // form field name used in POST
	Type   string    `json:"type"`   // input_text | input_file | multi_value_single_select | textarea
	Values []ghValue `json:"values"` // options for dropdowns/radios
}

type ghValue struct {
	Value json.RawMessage `json:"value"` // can be string or int (dropdown option IDs)
	Label string          `json:"label"`
}

// ValueStr returns the string form of Value regardless of underlying JSON type.
func (v ghValue) ValueStr() string {
	if len(v.Value) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return s
	}
	return string(v.Value)
}

// Company is an entry from data/companies.json
type Company struct {
	Name  string `json:"name"`
	Board string `json:"board"` // Greenhouse board token
}

// ── Job-board renderer (public apply form) types ─────────────────────────
// These mirror the JSON the Greenhouse job-board renderer (Remix app at
// boards.greenhouse.io) serves to the browser when a human opens an apply
// form. Reverse-engineered from the live site — see loader.go.

// loaderResponse is returned by the Remix data endpoint backing the public
// application form, e.g.
// GET /embed/job_app?for={board}&token={jobID}&_data=routes/embed.job_app
type loaderResponse struct {
	SubmitPath       string        `json:"submitPath"`
	ConfirmationPath string        `json:"confirmationPath"`
	URLToken         string        `json:"urlToken"`
	JobPostID        int64         `json:"jobPostId"`
	Internal         bool          `json:"internal"`
	JobPost          loaderJobPost `json:"jobPost"`
}

type loaderJobPost struct {
	Title       string       `json:"title"`
	CompanyName string       `json:"company_name"`
	Location    string       `json:"job_post_location"`
	Fingerprint string       `json:"fingerprint"` // server-issued token, echoed back at submit time
	Questions   []ghQuestion `json:"questions"`
}

// presignedTarget is one entry of the
// GET /uncacheable_attributes/presigned_fields?fields[]=<name> response:
// the S3 POST policy fields plus the object key template for one form field.
type presignedTarget struct {
	Fields map[string]string `json:"fields"`
	Key    string            `json:"key"` // contains {timestamp} and {unique_id} placeholders
}
