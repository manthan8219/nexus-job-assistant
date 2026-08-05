// Command companies-seed fills ~/.nexus/companies.db from multiple sources:
//
//  1. OpenJobs public dataset (hire countries + ATS links)
//
//  2. JobPilot embedded board lists (Greenhouse/Lever/Ashby/…)
//
//     go run ./cmd/companies-seed
//     go run ./cmd/companies-seed -country India
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
)

func main() {
	country := flag.String("country", "", "after seed, print companies for this country")
	limit := flag.Int("limit", 20, "max companies to print when -country is set")
	url := flag.String("url", companies.OpenJobsDefaultURL, "OpenJobs JSON URL")
	skipOpenJobs := flag.Bool("skip-openjobs", false, "only import JobPilot board lists")
	skipBoards := flag.Bool("skip-boards", false, "only import OpenJobs")
	skipYC := flag.Bool("skip-yc", false, "skip Y Combinator company directory import")
	flag.Parse()

	db, err := companies.OpenDefault()
	if err != nil {
		fail(err)
	}
	defer db.Close()

	if !*skipOpenJobs {
		fmt.Println("Source 1/4 — OpenJobs (public hire-country dataset)…")
		n, err := db.RefreshFromOpenJobs(*url)
		if err != nil {
			fail(err)
		}
		fmt.Printf("  upserted %d rows from OpenJobs\n", n)
	}
	if !*skipBoards {
		fmt.Println("Source 2/4 — JobPilot embedded ATS board lists…")
		n, err := db.ImportNexusEmbeddedBoards()
		if err != nil {
			fail(err)
		}
		fmt.Printf("  upserted %d rows from JobPilot boards\n", n)
	}
	fmt.Println("Source 3/4 — India priority employers (Microsoft, Google, Flipkart, …)…")
	n, err := db.ImportIndiaEmployers()
	if err != nil {
		fail(err)
	}
	fmt.Printf("  upserted %d India-priority employers\n", n)

	if !*skipYC {
		fmt.Println("Source 4/4 — Y Combinator company directory (startup-tagged, no ATS)…")
		n, err := db.RefreshFromYCombinator("")
		if err != nil {
			fail(err)
		}
		fmt.Printf("  upserted %d YC-backed companies\n", n)
	}

	total, _ := db.Count()
	fmt.Printf("Database now has %d companies\n", total)

	if *country == "" {
		fmt.Println("Try: go run ./cmd/companies-seed -country India")
		fmt.Println("Or open the Companies tab in JobPilot to search / add startups.")
		return
	}
	list, err := db.FindByCountryLimit(*country, *limit)
	if err != nil {
		fail(err)
	}
	count, _ := db.CountByCountry(*country)
	name, iso, _ := companies.NormalizeCountry(*country)
	fmt.Printf("\n%s (%s) — %d companies (showing up to %d):\n", name, iso, count, *limit)
	for _, c := range list {
		ats := c.ATS
		if ats == "" {
			ats = "—"
		}
		fmt.Printf("  %-28s  ats=%-14s  board=%s  src=%s\n", c.Name, ats, c.Board, c.Source)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "companies-seed: %v\n", err)
	os.Exit(1)
}
