// Package companies is a local employer database keyed by country footprint.
//
// Sources (not a single API):
//  1. OpenJobs public JSON — hire countries + ATS/career URLs
//  2. Nexus embedded board lists (Greenhouse, Lever, Ashby, Workday, …)
//  3. India priority employers (Microsoft, Google, Flipkart, …)
//  4. Manual inserts from the Companies tab (a) or Upsert API
//
// Store: ~/.nexus/companies.db (SQLite)
//
// Engine integration: RunOnce extracts countries from Config Target Locations
// (e.g. "Bengaluru, India" → IN) and merges companies.db ATS boards into each
// per-company provider via provider.BoardMerger — expanding beyond the small
// embedded lists while staying location-specific.
//
// Seed:
//
//	go run ./cmd/companies-seed
//	go run ./cmd/companies-seed -country India
package companies
