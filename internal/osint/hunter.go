package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type hunterEmailEntry struct {
	Value      string `json:"value"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Position   string `json:"position"`
	LinkedIn   string `json:"linkedin"`
	Confidence int    `json:"confidence"`
}

type hunterResponse struct {
	Data struct {
		Emails []hunterEmailEntry `json:"emails"`
	} `json:"data"`
	Errors []struct {
		Details string `json:"details"`
	} `json:"errors"`
}

func (f *Finder) hunterSearch(ctx context.Context, company, domain string) ([]Contact, error) {
	u := "https://api.hunter.io/v2/domain-search?" + url.Values{
		"domain":  {domain},
		"api_key": {f.hunterKey},
		"limit":   {"20"},
		"type":    {"personal"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid Hunter.io API key")
	}
	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, fmt.Errorf("Hunter.io quota exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hunter.io HTTP %d", resp.StatusCode)
	}

	var hr hunterResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(hr.Errors) > 0 {
		return nil, fmt.Errorf("%s", hr.Errors[0].Details)
	}

	var contacts []Contact
	for _, e := range hr.Data.Emails {
		if e.Value == "" {
			continue
		}
		name := e.FirstName
		if e.LastName != "" {
			if name != "" {
				name += " "
			}
			name += e.LastName
		}
		contacts = append(contacts, Contact{
			Company:    company,
			Domain:     domain,
			Name:       name,
			Title:      e.Position,
			Email:      e.Value,
			LinkedIn:   e.LinkedIn,
			Source:     "hunter",
			Confidence: e.Confidence,
		})
	}
	return contacts, nil
}
