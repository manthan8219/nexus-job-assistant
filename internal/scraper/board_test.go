package scraper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrapeBoardHTTP(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    int // number of jobs
		wantErr bool
	}{
		{
			name: "happy path returns jobs",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/scrape/board" {
					t.Errorf("path = %q, want /scrape/board", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("method = %q, want POST", r.Method)
				}
				var req boardRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if req.URL != "https://example.com/jobs" {
					t.Errorf("url = %q", req.URL)
				}
				if req.UseSession {
					t.Error("use_session should be false")
				}
				json.NewEncoder(w).Encode(boardResponse{Jobs: []BoardJob{
					{Title: "Engineer", Company: "Acme", Location: "Remote", ApplyURL: "https://example.com/jobs/1", Remote: true},
					{Title: "Developer", Company: "Beta", Location: "Berlin", ApplyURL: "https://example.com/jobs/2"},
				}})
			},
			want: 2,
		},
		{
			name: "service error string is surfaced",
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(boardResponse{Error: "CDP connect failed"})
			},
			wantErr: true,
		},
		{
			name: "non-2xx response returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name: "malformed json returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("{not valid json"))
			},
			wantErr: true,
		},
		{
			name: "empty jobs list returns no jobs and no error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(boardResponse{Jobs: []BoardJob{}})
			},
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			jobs, err := scrapeBoardHTTP(context.Background(), ts.URL, "https://example.com/jobs", "", nil, false)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(jobs) != tc.want {
				t.Errorf("got %d jobs, want %d", len(jobs), tc.want)
			}
		})
	}
}

func TestScrapeBoardHTTPBadHost(t *testing.T) {
	_, err := scrapeBoardHTTP(context.Background(), "http://127.0.0.1:0", "https://example.com/jobs", "", nil, false)
	if err == nil {
		t.Error("expected error for bad host, got nil")
	}
	if !strings.Contains(err.Error(), "scraper: board http") {
		t.Errorf("error should be wrapped as board http error, got %v", err)
	}
}

func TestCDPStatus(t *testing.T) {
	t.Run("reachable endpoint returns true", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/json/version" {
				t.Errorf("path = %q, want /json/version", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()
		if !cdpStatus(ts.URL) {
			t.Error("cdpStatus = false, want true for reachable CDP")
		}
	})

	t.Run("non-200 returns false", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()
		if cdpStatus(ts.URL) {
			t.Error("cdpStatus = true, want false for non-200")
		}
	})

	t.Run("unreachable returns false", func(t *testing.T) {
		if cdpStatus("http://127.0.0.1:0") {
			t.Error("cdpStatus = true, want false for unreachable")
		}
	})
}

func TestEnsureRunningNotInstalled(t *testing.T) {
	// When the scraper is not installed, EnsureRunning must return a clear
	// error without side effects. Running()/Installed() check real paths, but
	// if the venv is missing that is the not-installed branch.
	if Installed() {
		t.Skip("// reason: real scraper is installed in this environment")
	}
	err := EnsureRunning("", "")
	if err == nil {
		t.Error("expected error when scraper not installed")
	}
}
