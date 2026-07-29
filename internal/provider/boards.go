package provider

// NamedBoard is one employer board token used by per-company ATS clients.
// Board is a Greenhouse/Lever/Ashby slug, SmartRecruiters identifier, or a
// full Workday careers URL.
type NamedBoard struct {
	Name  string
	Board string
}

// BoardMerger lets the engine expand/replace the company list from companies.db
// for location-specific runs. MergeBoards must rebuild from the client's embedded
// base list each call (base ∪ extra), so repeated runs do not grow forever.
type BoardMerger interface {
	MergeBoards(extra []NamedBoard)
}
