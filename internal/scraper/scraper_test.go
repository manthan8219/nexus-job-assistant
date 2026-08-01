package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRootURL(t *testing.T) {
	cases := []struct {
		name    string
		website string
		want    string
		wantErr bool
	}{
		{"with scheme", "https://stripe.com", "https://stripe.com", false},
		{"without scheme", "stripe.com", "https://stripe.com", false},
		{"drops path", "https://stripe.com/careers", "https://stripe.com", false},
		{"http scheme kept", "http://example.com", "http://example.com", false},
		{"empty errors", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := rootURL(c.website)
			if (err != nil) != c.wantErr {
				t.Fatalf("rootURL(%q) err = %v; wantErr %v", c.website, err, c.wantErr)
			}
			if !c.wantErr && got != c.want {
				t.Errorf("rootURL(%q) = %q; want %q", c.website, got, c.want)
			}
		})
	}
}

func TestLooksLikeCareersPage(t *testing.T) {
	t.Run("200 with hiring signal", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><h1>We're hiring engineers</h1></html>"))
		}))
		defer ts.Close()
		ok, err := looksLikeCareersPage(ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true for 200 page with hiring signal")
		}
	})
	t.Run("404 not a careers page", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		ok, _ := looksLikeCareersPage(ts.URL)
		if ok {
			t.Error("expected false for 404")
		}
	})
	t.Run("non-html content type", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		}))
		defer ts.Close()
		ok, _ := looksLikeCareersPage(ts.URL)
		if ok {
			t.Error("expected false for non-html content type")
		}
	})
	t.Run("200 without signal", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><h1>Welcome to our blog</h1></html>"))
		}))
		defer ts.Close()
		ok, _ := looksLikeCareersPage(ts.URL)
		if ok {
			t.Error("expected false for page without career signals")
		}
	})
}

func TestDiscoverCareersURL_Found(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/careers" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>we are hiring, view all jobs</html>"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	got, err := DiscoverCareersURL(ts.URL)
	if err != nil {
		t.Fatalf("DiscoverCareersURL: %v", err)
	}
	if want := ts.URL + "/careers"; got != want {
		t.Errorf("DiscoverCareersURL = %q; want %q", got, want)
	}
}

func TestDiscoverCareersURL_FirstMatchWins(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>open position apply now</html>"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	got, err := DiscoverCareersURL(ts.URL)
	if err != nil {
		t.Fatalf("DiscoverCareersURL: %v", err)
	}
	if want := ts.URL + "/jobs"; got != want {
		t.Errorf("DiscoverCareersURL = %q; want %q", got, want)
	}
}

func TestDiscoverCareersURL_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	got, err := DiscoverCareersURL(ts.URL)
	if err != nil {
		t.Fatalf("DiscoverCareersURL: %v", err)
	}
	if got != "" {
		t.Errorf("DiscoverCareersURL = %q; want \"\" when nothing found", got)
	}
}

func TestBackendByID(t *testing.T) {
	if b := BackendByID("playwright"); b == nil || b.ID != "playwright" {
		t.Errorf("BackendByID(\"playwright\") = %v; want non-nil playwright", b)
	}
	if b := BackendByID("nonexistent"); b != nil {
		t.Errorf("BackendByID(\"nonexistent\") = %v; want nil", b)
	}
}

func TestSplitCmd(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"playwright install chromium", []string{"playwright", "install", "chromium"}},
		{"crawl4ai-setup", []string{"crawl4ai-setup"}},
		{"", nil},
		{"  trim  me ", []string{"trim", "me"}},
	}
	for _, c := range cases {
		got := splitCmd(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCmd(%q) = %v; want %v (len mismatch)", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCmd(%q)[%d] = %q; want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestCatalog_Integrity(t *testing.T) {
	if len(Catalog) < 3 {
		t.Fatalf("expected at least 3 backends in Catalog, got %d", len(Catalog))
	}
	seen := map[string]bool{}
	for _, b := range Catalog {
		if b.ID == "" {
			t.Error("catalog backend with empty ID")
		}
		if seen[b.ID] {
			t.Errorf("duplicate backend ID %q", b.ID)
		}
		seen[b.ID] = true
		if b.Name == "" || len(b.PipPackages) == 0 {
			t.Errorf("backend %q missing name or pip packages", b.ID)
		}
	}
}
