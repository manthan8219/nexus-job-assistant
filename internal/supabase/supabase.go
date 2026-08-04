// Package supabase connects Nexus to a managed Supabase project: Postgres via
// pgx for the relational store, and the Storage REST API for object files
// (resumes). A Client is built from config; Check() probes both Database and
// Storage so callers can automatically verify everything is reachable.
package supabase

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// DefaultBucket is where resume files are stored.
const DefaultBucket = "resumes"

// Client talks to a Supabase project.
//
// URL is the project URL (https://<ref>.supabase.co), ServiceKey is the
// service_role key (used for Storage; never logged), and DBURL is the
// Postgres connection string for pgx.
type Client struct {
	URL        string
	ServiceKey string
	DBURL      string
	http       *http.Client
}

// New builds a client from raw values. Empty URL disables Storage; empty DBURL
// disables the Database half (the check then reports that half as skipped).
func New(url, serviceKey, dbURL string) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		ServiceKey: serviceKey,
		DBURL:      dbURL,
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

// FromConfig builds a client from config, or nil when Supabase is not
// configured at all (no URL).
func FromConfig(cfg *config.Config) *Client {
	if cfg == nil {
		return nil
	}
	url := strings.TrimSpace(cfg.SupabaseURL)
	if url == "" {
		return nil
	}
	return New(url, strings.TrimSpace(cfg.SupabaseServiceKey), strings.TrimSpace(cfg.DatabaseURL))
}

// Configured reports whether a Supabase project URL is set.
func Configured(cfg *config.Config) bool {
	return cfg != nil && strings.TrimSpace(cfg.SupabaseURL) != ""
}

// CheckResult is the outcome of a health probe against a Supabase project.
type CheckResult struct {
	DatabaseOK   bool     `json:"databaseOK"`
	DatabaseErr  string   `json:"databaseErr,omitempty"`
	DatabaseSkip bool     `json:"databaseSkipped,omitempty"`
	StorageOK    bool     `json:"storageOK"`
	StorageErr   string   `json:"storageErr,omitempty"`
	StorageSkip  bool     `json:"storageSkipped,omitempty"`
	Buckets      []string `json:"buckets,omitempty"`
	Invites      []string `json:"resumeBucketMissing,omitempty"`
	ResumeBucket bool     `json:"resumeBucketOK"`
}

// OK reports whether every configured half is healthy.
func (r *CheckResult) OK() bool {
	return (r.DatabaseSkip || r.DatabaseOK) && (r.StorageSkip || r.StorageOK) && r.ResumeBucket
}

// String renders a one-line human summary.
func (r *CheckResult) String() string {
	db := "ok"
	if r.DatabaseSkip {
		db = "skipped (no DB URL)"
	} else if !r.DatabaseOK && r.DatabaseErr != "" {
		db = "FAIL: " + r.DatabaseErr
	}
	st := "ok"
	if r.StorageSkip {
		st = "skipped (no key)"
	} else if !r.StorageOK && r.StorageErr != "" {
		st = "FAIL: " + r.StorageErr
	}
	return fmt.Sprintf("database=%s\nstorage=%s\nresume-bucket=%v\nbuckets=%v", db, st, r.ResumeBucket, r.Buckets)
}

// Check probes the database (pgx ping) and storage (bucket list) and reports
// whether everything is reachable and the resume bucket exists. Automatically
// skips a half that is not configured.
func (c *Client) Check(ctx context.Context) *CheckResult {
	res := &CheckResult{}

	// Database
	if c.DBURL == "" {
		res.DatabaseSkip = true
	} else {
		db, err := sql.Open("pgx", c.DBURL)
		if err == nil {
			defer db.Close()
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := db.PingContext(pctx); err != nil {
				res.DatabaseErr = err.Error()
			} else {
				res.DatabaseOK = true
			}
		} else {
			res.DatabaseErr = err.Error()
		}
	}

	// Storage
	if c.ServiceKey == "" || c.URL == "" {
		res.StorageSkip = true
	} else {
		buckets, err := c.listBuckets(ctx)
		if err != nil {
			res.StorageErr = err.Error()
		} else {
			res.StorageOK = true
			res.Buckets = buckets
			for _, b := range buckets {
				if b == DefaultBucket {
					res.ResumeBucket = true
				}
			}
		}
	}
	return res
}

// listBuckets lists storage buckets via the authenticated Storage API.
func (c *Client) listBuckets(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/storage/v1/bucket", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.ServiceKey)
	req.Header.Set("apikey", c.ServiceKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list buckets HTTP %d: %s", resp.StatusCode, truncate(body, 160))
	}
	var buckets []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &buckets); err != nil {
		return nil, fmt.Errorf("decode buckets: %w", err)
	}
	out := make([]string, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b.ID)
	}
	return out, nil
}

// UploadResume stores a resume PDF under name in the default bucket.
func (c *Client) UploadResume(ctx context.Context, name string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/storage/v1/object/"+DefaultBucket+"/"+name, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.ServiceKey)
	req.Header.Set("apikey", c.ServiceKey)
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload resume: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("upload resume HTTP %d: %s", resp.StatusCode, truncate(body, 160))
	}
	return nil
}

// ResumeURL returns the direct public URL for a stored resume, or "" when the
// bucket is private and signed URLs are required (signed URLs are a follow-up).
func (c *Client) ResumeURL(name string) string {
	return c.URL + "/storage/v1/object/public/" + DefaultBucket + "/" + name
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
