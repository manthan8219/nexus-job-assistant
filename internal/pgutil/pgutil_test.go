package pgutil

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "url with password",
			dsn:  "postgresql://postgres:secret@db.example.com:5432/postgres",
			want: "postgresql://postgres:xxxxxx@db.example.com:5432/postgres",
		},
		{
			name: "url without password",
			dsn:  "postgresql://postgres@db.example.com:5432/postgres",
			want: "postgresql://postgres@db.example.com:5432/postgres",
		},
		{
			name: "non-url dsn stays",
			dsn:  "host=db.example.com user=postgres dbname=postgres",
			want: "host=db.example.com user=postgres dbname=postgres",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactDSN(tt.dsn); got != tt.want {
				t.Errorf("RedactDSN(%q) = %q; want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestRedactDSNNeverLeaksPassword(t *testing.T) {
	secret := "%WijpJ/eUh6fz3T"
	dsn := "postgresql://postgres:" + secret + "@db.nhrqgxwrizkfjlsypixq.supabase.co:5432/postgres"
	if got := RedactDSN(dsn); strings.Contains(got, secret) || strings.Contains(got, "%25Wi") {
		t.Errorf("RedactDSN leaked the password: %q", got)
	}
}

func TestWrapConnectErrorAddsHintAndWraps(t *testing.T) {
	dsn := "postgresql://postgres:%WijpJ@db.example.com:5432/postgres"
	err := errors.New("failed to parse as URL (invalid port \":%WijpJ\" after host)")
	wrapped := WrapConnectError(err, dsn)
	if wrapped == nil {
		t.Fatal("WrapConnectError returned nil")
	}
	msg := wrapped.Error()
	if !strings.Contains(msg, "URL-encode") || !strings.Contains(msg, "xxxxxx") {
		t.Errorf("expected hint + redacted dsn, got: %s", msg)
	}
	if !errors.Is(wrapped, err) {
		t.Error("WrapConnectError must wrap the original error (errors.Is)")
	}
	if !errors.Is(wrapped, ErrBadDSN) {
		t.Error("WrapConnectError must wrap ErrBadDSN")
	}
	if strings.Contains(msg, "%WijpJ") {
		t.Errorf("password leaked in wrapped error: %s", msg)
	}
}

func TestWrapConnectErrorNonURLError(t *testing.T) {
	dsn := "postgresql://postgres:secret@db.example.com:5432/postgres"
	err := errors.New("some unrelated failure")
	wrapped := WrapConnectError(err, dsn)
	if strings.Contains(wrapped.Error(), "hint:") {
		t.Errorf("no hint expected for a non-URL error, got: %s", wrapped.Error())
	}
	if !errors.Is(wrapped, err) {
		t.Error("non-URL error must still unwrap to the cause")
	}
}
