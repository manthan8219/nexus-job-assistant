package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwkEntry is one public key (kid -> private key) served by a JWKS test server.
type jwkEntry struct {
	kid string
	key any // *ecdsa.PrivateKey or *rsa.PrivateKey
}

// fakeJWKS serves a configurable JWKS document and counts hits.
type fakeJWKS struct {
	mu      sync.Mutex
	entries []jwkEntry
	hits    atomic.Int64
	srv     *httptest.Server
}

func newFakeJWKS(entries ...jwkEntry) *fakeJWKS {
	f := &fakeJWKS{entries: entries}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		keys := make([]map[string]any, 0, len(f.entries))
		for _, e := range f.entries {
			keys = append(keys, jwkJSON(e))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	return f
}

func (f *fakeJWKS) set(entries ...jwkEntry) {
	f.mu.Lock()
	f.entries = entries
	f.mu.Unlock()
}

func (f *fakeJWKS) close() { f.srv.Close() }

func (f *fakeJWKS) url() string { return f.srv.URL + "/.well-known/jwks.json" }

func jwkJSON(e jwkEntry) map[string]any {
	switch k := e.key.(type) {
	case *ecdsa.PrivateKey:
		pub := &k.PublicKey
		size := (pub.Curve.Params().BitSize + 7) / 8
		curve := pub.Curve.Params().Name
		return map[string]any{
			"kty": "EC", "crv": curve, "kid": e.kid, "alg": "ES256",
			"x": base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, size))),
			"y": base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, size))),
		}
	case *rsa.PrivateKey:
		pub := &k.PublicKey
		eb := big.NewInt(int64(pub.E)).Bytes()
		return map[string]any{
			"kty": "RSA", "kid": e.kid, "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(eb),
		}
	}
	return nil
}

func testESKey() *ecdsa.PrivateKey {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return k
}

// signClaims signs a token for sub/email with the given method/key + kid.
func signClaims(t *testing.T, key any, method jwt.SigningMethod, kid, sub, email string, life time.Duration) string {
	t.Helper()
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Audience:  jwt.ClaimStrings{DefaultAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(life)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email: email,
	}
	tok := jwt.NewWithClaims(method, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestVerifyWithJWKS(t *testing.T) {
	key1 := testESKey()
	key2 := testESKey()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	f := newFakeJWKS(
		jwkEntry{kid: "k1", key: key1},
		jwkEntry{kid: "k2", key: rsaKey},
	)
	defer f.close()

	v := NewJWKS(f.url(), "", "")
	signedES := signClaims(t, key1, jwt.SigningMethodES256, "k1", "user-1", "ada@example.com", time.Hour)
	signedRS := signClaims(t, rsaKey, jwt.SigningMethodRS256, "k2", "user-2", "bob@example.com", time.Hour)
	signedWrong := signClaims(t, key2, jwt.SigningMethodES256, "k1", "user-3", "eve@example.com", time.Hour)

	tests := []struct {
		name    string
		token   string
		want    User
		wantErr error
	}{
		{name: "valid ES256 token", token: signedES, want: User{ID: "user-1", Email: "ada@example.com"}},
		{name: "valid RS256 token", token: signedRS, want: User{ID: "user-2", Email: "bob@example.com"}},
		{name: "empty token", token: "", wantErr: ErrNoToken},
		{name: "garbage token", token: "not-a-jwt", wantErr: ErrBadToken},
		{name: "wrong signature", token: signedWrong, wantErr: ErrBadToken},
		{name: "unknown kid", token: signClaims(t, key1, jwt.SigningMethodES256, "nope", "u", "x@y.z", time.Hour), wantErr: ErrBadToken},
		{name: "expired token", token: signClaims(t, key1, jwt.SigningMethodES256, "k1", "u", "x@y.z", -time.Hour), wantErr: ErrExpired},
		{name: "hs256 token rejected", token: signClaims(t, []byte("s"), jwt.SigningMethodHS256, "k1", "u", "x@y.z", time.Hour), wantErr: ErrBadToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v.Verify(tt.token)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Verify(%q) = %+v, nil; want error", tt.token, got)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Verify(%q) error = %v; want errors.Is %v", tt.token, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify(%q) error = %v; want nil", tt.token, err)
			}
			if got != tt.want {
				t.Errorf("Verify(%q) = %+v; want %+v", tt.token, got, tt.want)
			}
		})
	}
}

func TestVerifyWithJWKSCachesKeyset(t *testing.T) {
	key := testESKey()
	f := newFakeJWKS(jwkEntry{kid: "k1", key: key})
	defer f.close()

	v := NewJWKS(f.url(), "", "")
	signed := signClaims(t, key, jwt.SigningMethodES256, "k1", "u", "x@y.z", time.Hour)

	for i := 0; i < 3; i++ {
		if _, err := v.Verify(signed); err != nil {
			t.Fatalf("verify #%d: %v", i+1, err)
		}
	}
	if got := f.hits.Load(); got != 1 {
		t.Errorf("jwks fetches = %d; want 1 (cached keyset)", got)
	}
}

func TestVerifyWithJWKSRefreshesAfterRotation(t *testing.T) {
	keyA := testESKey()
	keyB := testESKey()
	f := newFakeJWKS(jwkEntry{kid: "key-a", key: keyA})
	defer f.close()

	v := NewJWKS(f.url(), "", "")
	v.jwksTTL = time.Nanosecond // force a refetch when the kid is unknown

	// First verify fetches key-a.
	if _, err := v.Verify(signClaims(t, keyA, jwt.SigningMethodES256, "key-a", "u", "x@y.z", time.Hour)); err != nil {
		t.Fatalf("verify with key-a: %v", err)
	}
	// Rotate: the provider now serves only key-b.
	f.set(jwkEntry{kid: "key-b", key: keyB})
	if _, err := v.Verify(signClaims(t, keyB, jwt.SigningMethodES256, "key-b", "u", "x@y.z", time.Hour)); err != nil {
		t.Fatalf("verify after rotation to key-b: %v", err)
	}
	if got := f.hits.Load(); got != 2 {
		t.Errorf("jwks fetches = %d; want 2 (initial + rotation)", got)
	}
}

func TestVerifyWithJWKSServerErrors(t *testing.T) {
	v := NewJWKS("http://127.0.0.1:1/.well-known/jwks.json", "", "") // connection refused
	long := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIn0.eyJzdWIiOiJ1IiwiZXhwIjo5OTk5OTk5OTk5fQ.abc"
	if _, err := v.Verify(long); !errors.Is(err, ErrBadToken) {
		t.Errorf("Verify with unreachable jwks error = %v; want ErrBadToken", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	v2 := NewJWKS(bad.URL, "", "")
	if _, err := v2.Verify(long); !errors.Is(err, ErrBadToken) {
		t.Errorf("Verify with 500 jwks error = %v; want ErrBadToken", err)
	}
}

func TestNewFromEnvJWKS(t *testing.T) {
	key := testESKey()
	f := newFakeJWKS(jwkEntry{kid: "k1", key: key})
	defer f.close()

	t.Setenv("NEXUS_SUPABASE_JWT_SECRET", "")
	t.Setenv("NEXUS_SUPABASE_JWKS_URL", f.url())
	t.Setenv("NEXUS_SUPABASE_URL", "")
	t.Setenv("NEXUS_SUPABASE_JWT_AUD", "")

	v := NewFromEnv()
	if v == nil {
		t.Fatal("NewFromEnv() = nil; want a JWKS verifier")
	}
	if v.jwksURL != f.url() {
		t.Errorf("jwksURL = %q; want %q", v.jwksURL, f.url())
	}
	signed := signClaims(t, key, jwt.SigningMethodES256, "k1", "u", "x@y.z", time.Hour)
	if _, err := v.Verify(signed); err != nil {
		t.Errorf("NewFromEnv verifier rejected valid ES256 token: %v", err)
	}
}
