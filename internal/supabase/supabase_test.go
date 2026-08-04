package supabase

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStorage serves the Storage bucket-list endpoint so tests stay hermetic.
func fakeStorage(t *testing.T, buckets []string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bucket") {
			if status != http.StatusOK {
				http.Error(w, "boom", status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			body := "["
			for i, b := range buckets {
				if i > 0 {
					body += ","
				}
				body += fmt.Sprintf("{\"id\":\"%s\"}", b)
			}
			body += "]"
			w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCheck_StorageOKWithResumeBucket(t *testing.T) {
	srv := fakeStorage(t, []string{"resumes"}, http.StatusOK)
	defer srv.Close()

	c := New(srv.URL, "secret", "")
	res := c.Check(context.Background())
	if !res.StorageOK {
		t.Fatalf("storage not OK: %s", res.StorageErr)
	}
	if !res.ResumeBucket {
		t.Error("expected resume bucket present")
	}
	if !res.DatabaseSkip {
		t.Error("expected database skipped when no DB URL")
	}
	if !res.OK() {
		t.Errorf("OK() = false; want true (storage ok + db skipped + bucket present)")
	}
}

func TestCheck_StorageError(t *testing.T) {
	srv := fakeStorage(t, nil, http.StatusInternalServerError)
	defer srv.Close()

	c := New(srv.URL, "secret", "")
	res := c.Check(context.Background())
	if res.StorageOK {
		t.Error("expected storage NOT ok on 500")
	}
	if res.StorageErr == "" {
		t.Error("expected an error message on storage failure")
	}
	if res.OK() {
		t.Error("OK() = true; want false when storage fails")
	}
}

func TestCheck_StorageSkippedWithoutKey(t *testing.T) {
	c := New("https://example.supabase.co", "", "")
	res := c.Check(context.Background())
	if !res.StorageSkip {
		t.Error("expected storage skipped without a service key")
	}
}

func TestFromConfig_NilWhenNoURL(t *testing.T) {
	if FromConfig(nil) != nil {
		t.Error("FromConfig(nil) should be nil")
	}
	if Configured(nil) {
		t.Error("Configured(nil) should be false")
	}
}

func TestUploadResume(t *testing.T) {
	var got bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/object/resumes/") {
			got = true
			if r.Header.Get("Authorization") != "Bearer k" {
				t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	if err := c.UploadResume(context.Background(), "resume.pdf", []byte("%PDF")); err != nil {
		t.Fatalf("UploadResume: %v", err)
	}
	if !got {
		t.Error("expected an upload request to /object/resumes/")
	}
}
