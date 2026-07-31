// tailor generates a job-tailored, HR-agent-reviewed CV + cover letter kit
// for one job, and renders it to LaTeX + PDF.
//
// Usage:
//
//	go run ./cmd/tailor --jd job.txt --company Acme --title "Backend Engineer"
//	go run ./cmd/tailor --url <tracked-job-url>   # job already seen by Nexus
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/tailor"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

func main() {
	jdPath := flag.String("jd", "", "path to a job-description text file")
	jobURL := flag.String("url", "", "URL of a job Nexus has already seen (fills title/company/JD from the DB)")
	company := flag.String("company", "", "company name (overrides --url)")
	title := flag.String("title", "", "job title (overrides --url)")
	location := flag.String("location", "", "job location (optional)")
	remote := flag.Bool("remote", false, "mark the job as remote")
	resumePath := flag.String("resume", "", "resume path override (default: config resume_path)")
	rounds := flag.Int("rounds", 0, "max writer→HR review rounds (default: config or 3; 1 = no review loop)")
	outDir := flag.String("out", "", "output directory (default: ~/.nexus/tailored/<company-role>)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		exit("load config: %v", err)
	}
	if !cfg.AIAssist {
		exit("%s", "AI Assist is off — enable it in the TUI Config tab first")
	}

	job, err := buildJob(*jobURL, *jdPath, *company, *title, *location, *remote)
	if err != nil {
		exit("%v", err)
	}

	rPath := strings.TrimSpace(*resumePath)
	if rPath == "" {
		rPath = cfg.ResumePath
	}
	if strings.TrimSpace(rPath) == "" {
		exit("%s", "no resume — set resume_path in Config or pass --resume")
	}
	text, err := resume.ExtractText(rPath)
	if err != nil {
		exit("could not read resume: %v", err)
	}

	projects, err := workcontext.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: work context unavailable: %v\n", err)
	}

	fmt.Printf("⚡ Nexus Tailor — %s @ %s\n", job.Title, job.Company)
	if strings.TrimSpace(job.Description) == "" {
		fmt.Println("   (no job description — tailoring from title/company only)")
	}
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	out, err := tailor.Generate(ctx, cfg, tailor.Input{
		Job:             job,
		ResumePath:      rPath,
		ResumeText:      text,
		Projects:        projects,
		MaxRounds:       *rounds,
		OutDir:          *outDir,
		RegisterLibrary: true,
		Logf:            func(format string, args ...any) { fmt.Printf(format+"\n", args...) },
	})
	if err != nil {
		exit("%v", err)
	}

	fmt.Println()
	if out.Passed {
		fmt.Printf("✅ HR agent APPROVED in %d round(s) — ATS %d/100 · HR %d/100\n",
			out.Rounds, out.Review.ATSScore, out.Review.HRScore)
	} else {
		fmt.Printf("⚠️  HR agent never passed (%d rounds) — kept best draft: ATS %d/100 · HR %d/100\n",
			out.Rounds, out.Review.ATSScore, out.Review.HRScore)
	}
	fmt.Printf("   %s\n\n", out.Review.Summary)
	fmt.Println("📄 Kit saved to " + out.Dir)
	for name, p := range map[string]string{
		"resume.pdf": out.ResumePDF, "resume.tex": out.ResumeTeX,
		"cover_letter.pdf": out.CoverPDF, "cover_letter.tex": out.CoverTeX,
		"review.json": out.ReviewJSON,
	} {
		if p != "" {
			fmt.Printf("   %-18s %s\n", name, p)
		}
	}
	if out.PDFNote != "" {
		fmt.Println("   note: " + out.PDFNote)
	}
	fmt.Println("\nThe tailored CV is registered in your resume library — pick it in the TUI Resume tab when applying.")
}

// buildJob assembles the target job from flags and, when --url is given, the
// stored application record (which carries the fetched JD).
func buildJob(jobURL, jdPath, company, title, location string, remote bool) (provider.Job, error) {
	var job provider.Job
	if jobURL != "" {
		st, err := store.Open()
		if err != nil {
			return job, fmt.Errorf("open store: %w", err)
		}
		defer st.Close()
		apps, err := st.List()
		if err != nil {
			return job, fmt.Errorf("list applications: %w", err)
		}
		for _, a := range apps {
			if a.URL == jobURL {
				job = provider.Job{
					Title: a.Role, Company: a.Company, Location: a.Location,
					Remote: a.Remote, URL: a.URL, Provider: a.Provider,
					Description: a.Description,
				}
				break
			}
		}
		if job.URL == "" {
			return job, fmt.Errorf("job URL not found in the Nexus database — run the engine first, or pass --jd/--company/--title")
		}
	}
	if jdPath != "" {
		data, err := os.ReadFile(jdPath)
		if err != nil {
			return job, fmt.Errorf("read JD file: %w", err)
		}
		job.Description = string(data)
	}
	if title != "" {
		job.Title = title
	}
	if company != "" {
		job.Company = company
	}
	if location != "" {
		job.Location = location
	}
	if remote {
		job.Remote = true
	}
	if strings.TrimSpace(job.Title) == "" {
		return job, fmt.Errorf("job title is required — pass --title (or --url of a tracked job)")
	}
	if strings.TrimSpace(job.Company) == "" {
		return job, fmt.Errorf("company is required — pass --company (or --url of a tracked job)")
	}
	return job, nil
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
