package osint

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// roundTripFunc adapts a handler function to http.RoundTripper so tests can
// intercept every request a Finder makes without touching the network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResp(t *testing.T, status int, body string, r *http.Request) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

func TestHunterSearch(t *testing.T) {
	var gotQuery string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return jsonResp(t, 200, `{"data":{"emails":[
			{"value":"alice@acme.com","first_name":"Alice","last_name":"Smith","position":"Recruiter","linkedin":"in/alice","confidence":97},
			{"value":"bob@acme.com","first_name":"Bob","last_name":"","position":"","confidence":0},
			{"value":"","first_name":"Empty","last_name":"Row"}
		]}}`, r), nil
	})
	f := NewFinder("hunter-key", "")
	f.http = &http.Client{Transport: transport}

	contacts, err := f.hunterSearch(context.Background(), "Acme", "acme.com")
	if err != nil {
		t.Fatalf("hunterSearch: %v", err)
	}
	if !strings.Contains(gotQuery, "api_key=hunter-key") || !strings.Contains(gotQuery, "domain=acme.com") {
		t.Errorf("query = %q; want api_key and domain", gotQuery)
	}
	if len(contacts) != 2 {
		t.Fatalf("len(contacts) = %d; want 2 (empty value skipped)", len(contacts))
	}
	if contacts[0].Email != "alice@acme.com" || contacts[0].Confidence != 97 || contacts[0].Source != "hunter" {
		t.Errorf("contact[0] = %+v; want alice@acme.com/97/hunter", contacts[0])
	}
	if contacts[1].Name != "Bob" {
		t.Errorf("contact[1].Name = %q; want \"Bob\"", contacts[1].Name)
	}
}

func TestHunterSearch_Errors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"unauthorized", 401, `{}`, "invalid Hunter.io API key"},
		{"quota", 402, `{}`, "Hunter.io quota exceeded"},
		{"http error", 500, `{}`, "Hunter.io HTTP 500"},
		{"api error field", 200, `{"errors":[{"details":"limit reached"}]}`, "limit reached"},
		{"malformed json", 200, `not json`, "decode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResp(t, c.status, c.body, r), nil
			})
			f := NewFinder("k", "")
			f.http = &http.Client{Transport: transport}
			_, err := f.hunterSearch(context.Background(), "Acme", "acme.com")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestApolloSearch(t *testing.T) {
	var gotPath string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return jsonResp(t, 200, `{"people":[
			{"name":"Alice Smith","title":"Recruiter","email":"alice@acme.com","linkedin_url":"in/alice"},
			{"first_name":"Bob","last_name":"Jones","email":"bob@acme.com"},
			{"name":"No Email"}
		]}`, r), nil
	})
	f := NewFinder("", "apollo-key")
	f.http = &http.Client{Transport: transport}

	contacts, err := f.apolloSearch(context.Background(), "Acme")
	if err != nil {
		t.Fatalf("apolloSearch: %v", err)
	}
	if gotPath != "/api/v1/mixed_people/search" {
		t.Errorf("path = %q; want /api/v1/mixed_people/search", gotPath)
	}
	if len(contacts) != 3 {
		t.Fatalf("len(contacts) = %d; want 3", len(contacts))
	}
	if contacts[1].Name != "Bob Jones" || contacts[1].Source != "apollo" {
		t.Errorf("contact[1] = %+v; want Bob Jones / apollo", contacts[1])
	}
}

func TestApolloSearch_Errors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"unauthorized", 401, `{}`, "invalid Apollo.io API key"},
		{"http error", 500, `{}`, "Apollo.io HTTP 500"},
		{"api error field", 200, `{"error":"plan expired"}`, "plan expired"},
		{"malformed json", 200, `nope`, "decode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResp(t, c.status, c.body, r), nil
			})
			f := NewFinder("", "k")
			f.http = &http.Client{Transport: transport}
			_, err := f.apolloSearch(context.Background(), "Acme")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; want it to contain %q", err, c.wantErr)
			}
		})
	}
}

// ghTransport is a roundTripper that serves a scripted GitHub API.
func ghTransport(t *testing.T, cancelOnUser string, cancel func()) (roundTripFunc, *[]string) {
	t.Helper()
	var paths []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		body := ""
		switch r.URL.Path {
		case "/orgs/acme":
			body = `{"login":"acme"}`
		case "/orgs/acme/members":
			body = `[{"login":"alice"},{"login":"bob"},{"login":"carl"}]`
		case "/users/alice":
			body = `{"login":"alice","name":"Alice Smith","email":"alice@gmail.com","company":"@Acme","html_url":"github.com/alice"}`
		case "/users/bob":
			body = `{"login":"bob","name":"Bob Jones","email":"bob@acme.com","company":"Acme Inc"}`
		case "/users/carl":
			body = `{"login":"carl","name":"Carl King","email":"","company":""}`
		}
		if cancelOnUser != "" && r.URL.Path == "/users/"+cancelOnUser {
			// Cancel the run the moment the target user's profile is fetched,
			// so the mid-batch ctx.Done() path is exercised deterministically.
			cancel()
			return nil, context.Canceled
		}
		return jsonResp(t, 200, body, r), nil
	})
	return transport, &paths
}

func TestGitHubSearch(t *testing.T) {
	transport, _ := ghTransport(t, "", func() {})
	f := NewFinder("", "")
	f.http = &http.Client{Transport: transport}

	contacts, err := f.githubSearch(context.Background(), "Acme", "acme.com")
	if err != nil {
		t.Fatalf("githubSearch: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("len(contacts) = %d; want 3", len(contacts))
	}
	byEmail := map[string]Contact{}
	for _, c := range contacts {
		byEmail[c.Email] = c
	}
	alice := byEmail["alice@gmail.com"]
	if alice.EmailType != "personal" || alice.Confidence != 85 || alice.Title != "Acme" {
		t.Errorf("alice = %+v; want personal/85/title Acme (cleaned)", alice)
	}
	bob := byEmail["bob@acme.com"]
	if bob.EmailType != "work" || bob.Confidence != 80 {
		t.Errorf("bob = %+v; want work/80", bob)
	}
	carl := byEmail["carl.king@acme.com"]
	if carl.EmailType != "work" || carl.Confidence != 55 {
		t.Errorf("carl = %+v; want generated carl.king@acme.com work/55", carl)
	}
}

func TestGitHubSearch_OrgNotFoundFallsBackToNextSlug(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/orgs/") {
			return jsonResp(t, 404, `{"message":"Not Found"}`, r), nil
		}
		return jsonResp(t, 500, ``, r), nil
	})
	f := NewFinder("", "")
	f.http = &http.Client{Transport: transport}
	contacts, err := f.githubSearch(context.Background(), "Acme", "acme.com")
	if err != nil {
		t.Fatalf("githubSearch: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("len(contacts) = %d; want 0 when no org exists", len(contacts))
	}
}

func TestFetchGitHubOrgMembers_CancelledMidBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport, _ := ghTransport(t, "bob", cancel)
	f := NewFinder("", "")
	f.http = &http.Client{Transport: transport}

	contacts, err := fetchGitHubOrgMembers(ctx, f.http, "acme", "Acme", "acme.com")
	if err != nil {
		t.Fatalf("fetchGitHubOrgMembers: %v", err)
	}
	if len(contacts) != 1 || contacts[0].Email != "alice@gmail.com" {
		t.Errorf("contacts = %+v; want just alice (batch cancelled at bob)", contacts)
	}
}

func TestScraperSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/osint/contacts" {
			t.Errorf("path = %q; want /osint/contacts", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"contacts": []map[string]any{
			{"name": "Carol", "title": "Talent", "email": "carol@acme.com", "source": "theharvester", "confidence": 65},
			{"name": "Dan", "email": "dan@acme.com", "source": "emailfinder", "confidence": 60},
		}})
	}))
	defer srv.Close()

	f := NewFinder("", "")
	f.scraperURL = srv.URL
	contacts, err := f.scraperSearch(context.Background(), "Acme", "acme.com")
	if err != nil {
		t.Fatalf("scraperSearch: %v", err)
	}
	if len(contacts) != 2 || contacts[0].Email != "carol@acme.com" || contacts[0].Source != "theharvester" {
		t.Errorf("contacts = %+v; want carol + dan parsed", contacts)
	}
	if contacts[0].Company != "Acme" || contacts[0].Domain != "acme.com" {
		t.Errorf("contact[0] = %+v; want company/domain stamped", contacts[0])
	}
}

func TestScraperSearch_Errors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }, "500"},
		{"error field", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"error": "scraper down"})
		}, "scraper down"},
		{"malformed body", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "nope") }, "decode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			f := NewFinder("", "")
			f.scraperURL = srv.URL
			_, err := f.scraperSearch(context.Background(), "Acme", "acme.com")
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestFinderSearch_FullPipeline(t *testing.T) {
	gh, _ := ghTransport(t, "", func() {})
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"contacts": []map[string]any{
			{"name": "Carol", "email": "carol@acme.com", "source": "theharvester", "confidence": 65},
		}})
	}))
	defer scraper.Close()

	f := NewFinder("", "")
	f.http = &http.Client{Transport: gh}
	f.scraperURL = scraper.URL

	result := f.Search(context.Background(), "Acme", "acme.com")
	if len(result.Sources) != 3 {
		t.Errorf("sources = %v; want github, osint, pattern", result.Sources)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v; want none", result.Errors)
	}
	if len(result.Contacts) != 11 {
		// 3 github + 1 scraper + 7 patterns, deduped by email.
		t.Errorf("contacts = %d; want 11", len(result.Contacts))
	}
	seen := map[string]bool{}
	for _, c := range result.Contacts {
		if c.Email == "" || seen[c.Email] {
			t.Errorf("duplicate or empty email in Search results: %+v", c)
		}
		seen[c.Email] = true
	}
}

func TestFinderSearch_OneBadSourceDoesNotAbort(t *testing.T) {
	// GitHub 500s; scraper 500s; patterns still flow.
	gh := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResp(t, 500, ``, r), nil
	})
	scraper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer scraper.Close()

	f := NewFinder("", "")
	f.http = &http.Client{Transport: gh}
	f.scraperURL = scraper.URL

	result := f.Search(context.Background(), "Acme", "acme.com")
	if len(result.Contacts) != 7 {
		t.Errorf("contacts = %d; want 7 patterns despite all sources failing", len(result.Contacts))
	}
	if len(result.Errors) == 0 {
		t.Error("expected recorded errors from the failing sources")
	}
}
