// test-apply applies to one Greenhouse job and shows the result + DB status.
// Usage: go run ./cmd/test-apply --url <greenhouse-job-url>
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/data"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/greenhouse"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/lever"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func main() {
	jobURL := flag.String("url", "", "Greenhouse job URL (boards-api.greenhouse.io/...)")
	flag.Parse()

	if *jobURL == "" {
		fmt.Println("usage: go run ./cmd/test-apply --url <greenhouse-job-url>")
		os.Exit(1)
	}

	// ── Load config ────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("✗ load config:", err)
		os.Exit(1)
	}
	if cfg.Email == "" || cfg.FirstName == "" {
		fmt.Println("✗ profile incomplete — fill name/email in Config tab first")
		os.Exit(1)
	}
	if cfg.ResumePath == "" {
		fmt.Println("✗ no resume path set in Config")
		os.Exit(1)
	}

	// ── Parse board + job ID from URL ──────────────────────────────────────
	parts := strings.Split(*jobURL, "/")
	var board, jobID string
	if strings.Contains(*jobURL, "lever.co") {
		// https://jobs.lever.co/{board}/{id}/apply
		for i, p := range parts {
			if strings.HasSuffix(p, "lever.co") && i+2 < len(parts) {
				board = parts[i+1]
				jobID = parts[i+2]
				break
			}
		}
	} else {
		// https://boards-api.greenhouse.io/v1/boards/{board}/jobs/{id}
		for i, p := range parts {
			if p == "boards" && i+1 < len(parts) {
				board = parts[i+1]
			}
			if p == "jobs" && i+1 < len(parts) {
				jobID = parts[i+1]
			}
		}
	}
	if board == "" || jobID == "" {
		fmt.Println("✗ could not parse board/jobID from URL:", *jobURL)
		os.Exit(1)
	}

	profile := provider.Profile{
		FirstName:  cfg.FirstName,
		LastName:   cfg.LastName,
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		ResumePath: cfg.ResumePath,
		LinkedInID: cfg.LinkedInID,
		City:       cfg.City,
		MinSalary:  cfg.MinSalary,
	}

	// ── Build job object ───────────────────────────────────────────────────
	job := provider.Job{
		ID:       jobID,
		Provider: "greenhouse",
		Board:    board,
		URL:      *jobURL,
	}

	// ── Look up title/company from DB ──────────────────────────────────────
	st, err := store.Open()
	if err != nil {
		fmt.Println("✗ open store:", err)
		os.Exit(1)
	}
	apps, _ := st.List()
	for _, a := range apps {
		if a.URL == *jobURL {
			job.Title = a.Role
			job.Company = a.Company
			job.Location = a.Location
			break
		}
	}

	fmt.Printf("Applying to: %s @ %s\n", job.Title, job.Company)
	fmt.Printf("Resume path: %q\n", profile.ResumePath)
	fmt.Printf("Board: %s  JobID: %s\n\n", board, jobID)

	// ── Apply ──────────────────────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Pick provider based on URL
	var applyResult provider.ApplyResult
	if strings.Contains(*jobURL, "lever.co") {
		client, err := lever.New(data.LeverCompaniesJSON)
		if err != nil {
			fmt.Println("✗ init lever:", err)
			os.Exit(1)
		}
		applyResult, err = client.Apply(ctx, job, profile)
		if err != nil {
			fmt.Println("✗ apply error:", err)
			os.Exit(1)
		}
	} else {
		client, err := greenhouse.New(data.CompaniesJSON)
		if err != nil {
			fmt.Println("✗ init greenhouse:", err)
			os.Exit(1)
		}
		applyResult, err = client.Apply(ctx, job, profile)
		if err != nil {
			fmt.Println("✗ apply error:", err)
			os.Exit(1)
		}
	}
	result := applyResult
	_ = result
	fmt.Printf("Result:  %s\n", result.Status)
	if result.Reason != "" {
		fmt.Printf("Reason:  %s\n", result.Reason)
	}

	// ── Update DB status ───────────────────────────────────────────────────
	now := time.Now()
	_ = st.Insert(store.Application{
		Provider:  "greenhouse",
		Company:   job.Company,
		Role:      job.Title,
		URL:       job.URL,
		Status:    store.Status(result.Status),
		Reason:    result.Reason,
		AppliedAt: now,
		Location:  job.Location,
	})

	// ── Verify in DB ───────────────────────────────────────────────────────
	apps2, _ := st.List()
	for _, a := range apps2 {
		if a.URL == *jobURL {
			fmt.Printf("\nDB record:\n")
			fmt.Printf("  status:     %s\n", a.Status)
			fmt.Printf("  applied_at: %s\n", a.AppliedAt.Format(time.RFC3339))
			fmt.Printf("  reason:     %s\n", a.Reason)
			break
		}
	}
}
