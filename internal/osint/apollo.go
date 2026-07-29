package osint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type apolloRequest struct {
	APIKey       string   `json:"api_key"`
	QOrgName     string   `json:"q_organization_name"`
	PersonTitles []string `json:"person_titles"`
	PerPage      int      `json:"per_page"`
	Page         int      `json:"page"`
}

type apolloPerson struct {
	Name        string `json:"name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Title       string `json:"title"`
	Email       string `json:"email"`
	LinkedInURL string `json:"linkedin_url"`
}

type apolloResponse struct {
	People []apolloPerson `json:"people"`
	Error  string         `json:"error"`
}

func (f *Finder) apolloSearch(ctx context.Context, company string) ([]Contact, error) {
	payload := apolloRequest{
		APIKey:   f.apolloKey,
		QOrgName: company,
		PersonTitles: []string{
			"recruiter", "talent", "HR", "human resources",
			"talent acquisition", "people operations",
			"hiring manager", "talent partner",
		},
		PerPage: 20,
		Page:    1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.apollo.io/api/v1/mixed_people/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search/1.0)")

	resp, err := f.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid Apollo.io API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Apollo.io HTTP %d", resp.StatusCode)
	}

	var ar apolloResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if ar.Error != "" {
		return nil, fmt.Errorf("%s", ar.Error)
	}

	var contacts []Contact
	for _, p := range ar.People {
		name := p.Name
		if name == "" {
			name = strings.TrimSpace(p.FirstName + " " + p.LastName)
		}
		contacts = append(contacts, Contact{
			Company:  company,
			Name:     name,
			Title:    p.Title,
			Email:    p.Email,
			LinkedIn: p.LinkedInURL,
			Source:   "apollo",
		})
	}
	return contacts, nil
}
