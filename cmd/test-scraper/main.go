// test-scraper hits the local scraper service with one company and prints results.
// Usage: go run ./cmd/test-scraper [--url https://linear.app/careers] [--company Linear]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

func main() {
	url := flag.String("url", "https://linear.app/careers", "career page URL to scrape")
	company := flag.String("company", "Linear", "company name")
	title := flag.String("title", "", "filter by title keyword (optional)")
	verbose := flag.Bool("v", false, "print raw JSON from service")
	flag.Parse()

	// 1. Check service health
	if !scraper.Running() {
		fmt.Println("✗ scraper service not running on", scraper.BaseURL)
		fmt.Println("  start: cd ~/.nexus/scraper && uvicorn main:app --port 8765")
		os.Exit(1)
	}

	// print active backend
	resp, _ := http.Get(scraper.BaseURL + "/health")
	if resp != nil {
		var h map[string]any
		json.NewDecoder(resp.Body).Decode(&h)
		resp.Body.Close()
		fmt.Printf("● service up  backend=%v  model=%v\n\n", h["backend"], h["model"])
	}

	// 2. Build request
	keywords := []string{}
	if *title != "" {
		keywords = []string{*title}
	}
	reqBody, _ := json.Marshal(map[string]any{
		"url":            *url,
		"company":        *company,
		"title_keywords": keywords,
	})

	fmt.Printf("  scraping %s → %s\n\n", *company, *url)

	// 3. POST /scrape
	httpResp, err := http.Post(scraper.BaseURL+"/scrape", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		fmt.Println("✗ request failed:", err)
		os.Exit(1)
	}
	defer httpResp.Body.Close()

	raw, _ := io.ReadAll(httpResp.Body)

	if *verbose {
		fmt.Println("── raw response ──────────────────────────────")
		var pretty bytes.Buffer
		json.Indent(&pretty, raw, "", "  ")
		fmt.Println(pretty.String())
		fmt.Println("──────────────────────────────────────────────")
	}

	// 4. Parse
	var result struct {
		Company string `json:"company"`
		Backend string `json:"backend"`
		Error   string `json:"error"`
		Jobs    []struct {
			Title      string `json:"title"`
			Location   string `json:"location"`
			Department string `json:"department"`
			ApplyURL   string `json:"apply_url"`
			Remote     bool   `json:"remote"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		fmt.Println("✗ decode error:", err)
		fmt.Println("raw:", string(raw))
		os.Exit(1)
	}

	if result.Error != "" {
		fmt.Println("✗ scraper error:", result.Error)
		os.Exit(1)
	}

	if len(result.Jobs) == 0 {
		fmt.Println("  0 jobs returned")
		fmt.Println("  try: go run ./cmd/test-scraper --v to see raw LLM output")
		os.Exit(0)
	}

	fmt.Printf("  found %d jobs (backend: %s):\n\n", len(result.Jobs), result.Backend)
	for i, j := range result.Jobs {
		loc := j.Location
		if loc == "" {
			loc = "—"
		}
		remote := ""
		if j.Remote {
			remote = " [remote]"
		}
		dept := ""
		if j.Department != "" {
			dept = "  · " + j.Department
		}
		fmt.Printf("  %2d. %s%s\n      %s%s\n      %s\n\n", i+1, j.Title, dept, loc, remote, j.ApplyURL)
	}
}
