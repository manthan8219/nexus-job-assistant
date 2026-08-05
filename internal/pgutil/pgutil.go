// Package pgutil holds tiny helpers shared by the Postgres-backed store and
// settings layers: password-redacted DSNs for logging, and actionable wrap
// errors for the classic "connection string broken" failure on deployment
// platforms (an un-url-encoded password in NEXUS_DATABASE_URL).
package pgutil

import (
	"errors"
	"fmt"
	"strings"
)

// ErrBadDSN is wrapped by every error WrapConnectError returns, letting
// callers branch on the failure class rather than string matching.
var ErrBadDSN = errors.New("pgutil: bad dsn")

// sanitizedError carries a safe, fully redacted message while still unwrapping
// to the original cause so errors.Is/errors.As keep working.
type sanitizedError struct {
	msg   string
	cause error
}

func (e *sanitizedError) Error() string { return e.msg }
func (e *sanitizedError) Unwrap() error { return e.cause }

// passwordOf extracts the password from a postgresql:// URL DSN, or "" when
// there is none / the DSN is not a URL form.
func passwordOf(dsn string) string {
	scheme := strings.Index(dsn, "://")
	if scheme < 0 {
		return ""
	}
	rest := dsn[scheme+3:]
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return ""
	}
	userinfo := rest[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 {
		return ""
	}
	return userinfo[colon+1:]
}

// RedactDSN returns dsn with its password replaced by "xxxxxx" so it is safe
// to print in logs and errors. Non-URL (keyword=value) DSNs are returned
// unchanged because they carry no obvious password marker this package can
// reliably redact.
func RedactDSN(dsn string) string {
	pwd := passwordOf(dsn)
	if pwd == "" {
		return dsn
	}
	return strings.Replace(dsn, ":"+pwd+"@", ":xxxxxx@", 1)
}

// WrapConnectError annotates a Postgres parse/connect failure with the
// (redacted) DSN. The original driver error string is scanned and any
// password fragment it embedded is also redacted, so the returned error never
// leaks credentials. When the failure looks like a broken URL - invalid
// escape, invalid port, or a host that did not resolve - it appends the
// classic fix: URL-encode special characters in the password. The result
// unwraps to the original error (errors.Is works).
func WrapConnectError(err error, dsn string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if pwd := passwordOf(dsn); len(pwd) >= 4 {
		// Redact the full password and its prefix up to the first delimiter -
		// pgx truncates the value inside its "invalid port" detail, so a
		// partial fragment can otherwise leak.
		msg = strings.ReplaceAll(msg, pwd, "xxxxxx")
		for _, c := range []string{"/", ":", "@", "?", "#"} {
			if i := strings.Index(pwd, c); i >= 4 {
				msg = strings.ReplaceAll(msg, pwd[:i], "xxxxxx")
				break
			}
		}
	}

	base := fmt.Errorf("postgres connect (%s): %s", RedactDSN(dsn), msg)
	low := strings.ToLower(msg)
	if strings.Contains(low, "invalid port") ||
		strings.Contains(low, "invalid url escape") ||
		strings.Contains(low, "no such host") ||
		strings.Contains(low, "hostname resolving") {
		base = fmt.Errorf("%v\nhint: the password has special characters (e.g. %% / @ :) that must be"+
			" URL-encoded in the database URL - replace %% with %%25, / with %%2F, @ with %%40, : with %%3A", base)
	}
	return &sanitizedError{msg: base.Error(), cause: fmt.Errorf("%w: %w", ErrBadDSN, err)}
}
