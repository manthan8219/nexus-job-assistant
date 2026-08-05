package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "super-secret-supabase-jwt-value"

// signToken issues an HS256 token with the given claims and lifetime.
func signToken(t *testing.T, secret, issuer, aud string, life time.Duration) string {
	t.Helper()
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(now.Add(life)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email: "ada@example.com",
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func TestNewFromEnv(t *testing.T) {
	t.Run("disabled when secret unset", func(t *testing.T) {
		t.Setenv("NEXUS_SUPABASE_JWT_SECRET", "")
		t.Setenv("NEXUS_SUPABASE_URL", "")
		t.Setenv("NEXUS_SUPABASE_JWT_AUD", "")
		if got := NewFromEnv(); got != nil {
			t.Errorf("NewFromEnv() = %v; want nil when secret unset", got)
		}
	})

	t.Run("enabled with secret and URL derives issuer", func(t *testing.T) {
		t.Setenv("NEXUS_SUPABASE_JWT_SECRET", testSecret)
		t.Setenv("NEXUS_SUPABASE_URL", "https://abc.supabase.co/")
		t.Setenv("NEXUS_SUPABASE_JWT_AUD", "")
		v := NewFromEnv()
		if v == nil {
			t.Fatal("NewFromEnv() = nil; want verifier")
		}
		if v.issuer != "https://abc.supabase.co/auth/v1" {
			t.Errorf("issuer = %q; want https://abc.supabase.co/auth/v1", v.issuer)
		}
		if v.audience != DefaultAudience {
			t.Errorf("audience = %q; want %q", v.audience, DefaultAudience)
		}
	})
}

func TestVerifierVerify(t *testing.T) {
	issuer := "https://abc.supabase.co/auth/v1"
	tests := []struct {
		name    string
		v       *Verifier
		token   string
		want    User
		wantErr error
	}{
		{name: "valid token", v: New(testSecret, issuer, ""), token: signToken(t, testSecret, issuer, DefaultAudience, time.Hour),
			want: User{ID: "user-123", Email: "ada@example.com"}},
		{name: "valid token with full_name metadata", v: New(testSecret, issuer, ""),
			token: func() string {
				raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "user-7",
						Issuer:    issuer,
						Audience:  jwt.ClaimStrings{DefaultAudience},
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
					Email:        "bob@example.com",
					UserMetadata: map[string]any{"full_name": "Bob Builder"},
				}).SignedString([]byte(testSecret))
				if err != nil {
					t.Fatalf("sign token: %v", err)
				}
				return raw
			}(),
			want: User{ID: "user-7", Email: "bob@example.com", Name: "Bob Builder"}},
		{name: "nil verifier", v: nil, token: "x", wantErr: ErrNoVerifier},
		{name: "empty token", v: New(testSecret, issuer, ""), token: "", wantErr: ErrNoToken},
		{name: "garbage token", v: New(testSecret, issuer, ""), token: "not-a-jwt", wantErr: ErrBadToken},
		{name: "wrong secret", v: New("another-secret", issuer, ""),
			token: signToken(t, testSecret, issuer, DefaultAudience, time.Hour), wantErr: ErrBadToken},
		{name: "expired token", v: New(testSecret, issuer, ""),
			token: signToken(t, testSecret, issuer, DefaultAudience, -time.Hour), wantErr: ErrExpired},
		{name: "wrong audience", v: New(testSecret, issuer, ""),
			token: signToken(t, testSecret, issuer, "other-aud", time.Hour), wantErr: ErrBadToken},
		{name: "wrong issuer", v: New(testSecret, issuer, ""),
			token:   signToken(t, testSecret, "https://evil.supabase.co/auth/v1", DefaultAudience, time.Hour),
			wantErr: ErrBadToken},
		{name: "custom audience accepted", v: New(testSecret, issuer, "my-app"),
			token: signToken(t, testSecret, issuer, "my-app", time.Hour),
			want:  User{ID: "user-123", Email: "ada@example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.v.Verify(tt.token)
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

func TestVerifierVerifyRejectsNoneAlgorithm(t *testing.T) {
	v := New(testSecret, "", "")
	claims := jwt.RegisteredClaims{Subject: "user-1"}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none-alg token: %v", err)
	}
	if _, err := v.Verify(raw); !errors.Is(err, ErrBadToken) {
		t.Errorf("Verify(none-alg) error = %v; want ErrBadToken", err)
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "standard bearer", header: "Bearer abc.def.ghi", want: "abc.def.ghi"},
		{name: "bearer with surrounding spaces", header: "  Bearer abc  ", want: "abc"},
		{name: "missing header", header: "", want: ""},
		{name: "basic scheme ignored", header: "Basic abc===", want: ""},
		{name: "lowercase bearer ignored", header: "bearer abc", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/config", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if got := BearerToken(req); got != tt.want {
				t.Errorf("BearerToken() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestUserContext(t *testing.T) {
	want := User{ID: "user-1", Email: "a@b.c"}

	if _, ok := FromContext(context.Background()); ok {
		t.Error("FromContext(empty) ok = true; want false")
	}

	ctx := WithUser(context.Background(), want)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext(with user) ok = false; want true")
	}
	if got != want {
		t.Errorf("FromContext() = %+v; want %+v", got, want)
	}
}
