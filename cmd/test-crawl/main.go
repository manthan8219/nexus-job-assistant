// test-crawl scrapes one company via the careerscraper service and inserts into DB.
// Usage: go run ./cmd/test-crawl --company Linear --url https://linear.app/careers
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/scraper"
	"github.com/manthanmanthan/nexus/internal/store"
)

func main() {
	company := flag.String("company", "Linear", "company name")
	url := flag.String("url", "https://linear.app/careers", "career page URL")
	flag.Parse()

	if !scraper.Running() {
		fmt.Println("✗ scraper service not running on", scraper.BaseURL)
		os.Exit(1)
	}

	st, err := store.Open()
	if err != nil {
		fmt.Println("✗ open store:", err)
		os.Exit(1)
	}

	// ── hit scraper service ───────────────────────────────────────────────────
	type reqBody struct {
		URL      string   `json:"url"`
		Company  string   `json:"company"`
		Keywords []string `json:"title_keywords"`
	}
	type jobResult struct {
		Title    string `json:"title"`
		Company  string `json:"company"`
		Location string `json:"location"`
		ApplyURL string `json:"apply_url"`
		Remote   bool   `json:"remote"`
	}
	type respBody struct {
		Jobs    []jobResult `json:"jobs"`
		Backend string      `json:"backend"`
		Error   string      `json:"error"`
	}

	body, _ := json.Marshal(reqBody{URL: *url, Company: *company, Keywords: []string{}})
	fmt.Printf("POST %s/scrape\n", scraper.BaseURL)
	httpResp, err := http.Post(scraper.BaseURL+"/scrape", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Println("✗ scraper request failed:", err)
		os.Exit(1)
	}
	defer httpResp.Body.Close()

	rawBytes, _ := io.ReadAll(httpResp.Body)
	preview := string(rawBytes)
	if len(preview) > 300 {
		preview = preview[:300] + "..."
	}
	fmt.Printf("raw response: %s\n\n", preview)

	var r respBody
	if err := json.Unmarshal(rawBytes, &r); err != nil {
		fmt.Println("✗ decode error:", err)
		os.Exit(1)
	}
	if r.Error != "" {
		fmt.Println("✗ scraper error:", r.Error)
		os.Exit(1)
	}

	fmt.Printf("● backend: %s  found: %d jobs\n\n", r.Backend, len(r.Jobs))

	// ── insert into store ─────────────────────────────────────────────────────
	inserted, skipped := 0, 0
	for _, j := range r.Jobs {
		applyURL := strings.TrimSpace(j.ApplyURL)
		if applyURL == "" {
			applyURL = *url
		}
		exists, _ := st.Exists(applyURL)
		if exists {
			skipped++
			continue
		}
		app := store.Application{
			Provider:  "careerscraper",
			Company:   j.Company,
			Role:      j.Title,
			URL:       applyURL,
			Status:    store.StatusSkipped,
			Reason:    "discovered via career page crawl",
			Location:  j.Location,
			Remote:    j.Remote,
			AppliedAt: time.Now(),
		}
		if err := st.Insert(app); err != nil {
			fmt.Printf("  ✗ insert %q: %v\n", j.Title, err)
		} else {
			inserted++
			fmt.Printf("  + %s  |  %s  |  %s\n", j.Title, j.Location, applyURL)
		}
	}

	fmt.Printf("\ninserted: %d  already existed: %d\n", inserted, skipped)

	// ── verify from DB ────────────────────────────────────────────────────────
	apps, _ := st.List()
	count := 0
	for _, a := range apps {
		if a.Provider == "careerscraper" {
			count++
		}
	}
	fmt.Printf("total careerscraper rows in DB: %d\n", count)

	shown := 0
	for _, a := range apps {
		if a.Provider != "careerscraper" {
			continue
		}
		fmt.Printf("  · %s @ %s\n", a.Role, a.Company)
		shown++
		if shown >= 5 {
			break
		}
	}
}
