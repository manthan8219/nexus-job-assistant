// Command e2e-seed writes deterministic fixture applications into a Nexus
// store so the frontend contract/E2E suites can exercise the jobs and
// outreach surfaces without hitting real job boards.
//
// Usage:
//
//	go run ./cmd/e2e-seed -db /tmp/nexus-e2e-home/.nexus/applications.db
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func main() {
	var dbPath string
	flag.StringVar(&dbPath, "db", "", "path to applications.db to seed (required)")
	flag.Parse()
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "usage: e2e-seed -db /path/to/applications.db")
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	st, err := store.OpenAt(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	now := time.Now().UTC()
	apps := []store.Application{
		{
			Provider: "greenhouse", Company: "Acme Health", Role: "Cardiologist",
			URL: "https://boards.greenhouse.io/acmehealth/1", Status: store.StatusQueued,
			Reason: "awaiting your approval", AppliedAt: now.Add(-10 * time.Minute),
			Location: "Remote", Remote: true, PostedAt: now.Add(-2 * time.Hour),
			FitScore: 92, FitSummary: "strong match", Approved: true,
		},
		{
			Provider: "lever", Company: "Medcorp", Role: "Staff Engineer",
			URL: "https://jobs.lever.co/medcorp/2", Status: store.StatusQueued,
			Reason: "awaiting your approval", AppliedAt: now.Add(-12 * time.Minute),
			Location: "Berlin, Germany", Remote: false, PostedAt: now.Add(-3 * time.Hour),
			FitScore: 87, FitSummary: "good match",
		},
		{
			Provider: "ashby", Company: "Acme Health", Role: "Registered Nurse",
			URL: "https://jobs.ashbyhq.com/acmehealth/3", Status: store.StatusApplied,
			AppliedAt: now.Add(-6 * time.Hour), Location: "Remote", Remote: true,
			PostedAt: now.Add(-2 * time.Hour), FitScore: 81, FitSummary: "solid",
		},
		{
			Provider: "remoteok", Company: "Designio", Role: "Product Designer",
			URL: "https://remoteok.io/designio/4", Status: store.StatusApplied,
			AppliedAt: now.Add(-2 * time.Hour), Location: "London, UK", Remote: true,
			PostedAt: now.Add(-24 * time.Hour), FitScore: 76,
			Outcome: store.OutcomeInterview, OutcomeAt: now.Add(-1 * time.Hour),
		},
		{
			Provider: "hackernews", Company: "Acme Health", Role: "Data Analyst",
			URL: "https://news.ycombinator.com/jobs/5", Status: store.StatusSkipped,
			Reason: "fit below floor", AppliedAt: now.Add(-3 * time.Hour),
			Location: "Remote", Remote: true, PostedAt: now.Add(-30 * time.Hour),
		},
		{
			Provider: "remotive", Company: "Medcorp", Role: "Frontend Engineer",
			URL: "https://remotive.com/medcorp/6", Status: store.StatusFailed,
			Reason: "form captcha stop", AppliedAt: now.Add(-4 * time.Hour),
			Location: "Amsterdam, Netherlands", Remote: true, PostedAt: now.Add(-36 * time.Hour),
		},
	}

	inserted := 0
	for _, a := range apps {
		if err := st.Insert(a); err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", a.Role, err)
			os.Exit(1)
		}
		inserted++
	}
	fmt.Printf("seeded %d applications into %s\n", inserted, dbPath)
}
