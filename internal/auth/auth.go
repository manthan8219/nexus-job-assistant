// Package auth verifies identity tokens (JWTs) issued by an identity provider
// (Supabase Auth) and exposes the authenticated User on the request context.
//
// Nexus does not store passwords or sessions: the provider issues short-lived
// access tokens, the API verifies their signature and claims, and the stable
// `sub` claim becomes the user ID that owns every byte of that user's data
// (per-user config, databases, engine runs). Tokens arrive as
// `Authorization: Bearer <token>` from the frontend SDK.
//
// When no provider is configured (NEXUS_SUPABASE_JWT_SECRET unset) there is no
// Verifier and the API keeps the legacy unauthenticated single-user behavior so
// local TUI/CLI development and docker-compose run untouched.
//
// Supabase signs access tokens with HS256 using the project's JWT secret; the
// issuer is "<project>.supabase.co/auth/v1" and the audience is the project's
// api.aud setting ("authenticated" by default).
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Sentinel errors callers can branch on with errors.Is.
var (
	// ErrNoVerifier means auth is not configured; no token is accepted.
	ErrNoVerifier = errors.New("auth: no verifier configured")
	// ErrNoToken means the request carried no bearer token.
	ErrNoToken = errors.New("auth: missing bearer token")
	// ErrBadToken means the token failed signature or claim validation.
	ErrBadToken = errors.New("auth: invalid token")
	// ErrExpired means the token is past its expiry.
	ErrExpired = errors.New("auth: token expired")
)

// DefaultAudience is the Supabase audience for user access tokens (the
// project's api.aud setting, "authenticated" unless the project was changed).
const DefaultAudience = "authenticated"

// User is the authenticated identity attached to a verified request.
type User struct {
	ID    string // stable user ID — the JWT `sub` claim
	Email string
	Name  string
}

// Claims is the identity-token claims Nexus reads. RegisteredClaims carries
// the standard sub/iss/aud/exp fields; Email and UserMetadata are Supabase
// user-token fields used for display.
type Claims struct {
	jwt.RegisteredClaims
	Email        string         `json:"email"`
	UserMetadata map[string]any `json:"user_metadata,omitempty"`
}

// Verifier validates identity-token signatures and claims for one identity
// provider project. A nil *Verifier means auth is not configured.
type Verifier struct {
	secret   []byte
	issuer   string
	audience string
}

// New returns a Verifier validating HS256-signed tokens for the given
// audience (empty means DefaultAudience). A non-empty issuer enables the
// issuer claim check.
func New(secret, issuer, audience string) *Verifier {
	if audience == "" {
		audience = DefaultAudience
	}
	return &Verifier{secret: []byte(secret), issuer: issuer, audience: audience}
}

// NewFromEnv builds a Verifier from the standard Nexus env vars, or returns
// nil when auth is not enabled:
//
//	NEXUS_SUPABASE_JWT_SECRET (required — enables auth when set)
//	NEXUS_SUPABASE_URL        (optional — enables the issuer claim check)
//	NEXUS_SUPABASE_JWT_AUD    (optional — defaults to DefaultAudience)
func NewFromEnv() *Verifier {
	secret := os.Getenv("NEXUS_SUPABASE_JWT_SECRET")
	if secret == "" {
		return nil
	}
	issuer := strings.TrimSuffix(os.Getenv("NEXUS_SUPABASE_URL"), "/")
	if issuer != "" && !strings.HasSuffix(issuer, "/auth/v1") {
		issuer += "/auth/v1"
	}
	return New(secret, issuer, os.Getenv("NEXUS_SUPABASE_JWT_AUD"))
}

// Verify validates the signature and claims of tokenString and returns the
// authenticated User, or an errors.Is-compatible sentinel error.
func (v *Verifier) Verify(tokenString string) (User, error) {
	if v == nil {
		return User{}, ErrNoVerifier
	}
	if tokenString == "" {
		return User{}, ErrNoToken
	}

	claims := &Claims{}
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	}
	if v.issuer != "" {
		opts = append(opts, jwt.WithIssuer(v.issuer))
	}
	opts = append(opts, jwt.WithAudience(v.audience))

	tok, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return v.secret, nil
	}, opts...)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return User{}, ErrExpired
		}
		return User{}, fmt.Errorf("%w: %v", ErrBadToken, err)
	}
	if tok == nil || !tok.Valid {
		return User{}, ErrBadToken
	}

	var name string
	if meta, ok := claims.UserMetadata["full_name"].(string); ok {
		name = meta
	} else if n, ok := claims.UserMetadata["name"].(string); ok {
		name = n
	}
	return User{ID: claims.Subject, Email: claims.Email, Name: name}, nil
}

// BearerToken extracts the token from the request's Authorization header.
// Only the "Bearer " scheme is honored; anything else is treated as absent.
// Leading/trailing whitespace around the scheme and token is tolerated
// (transport parsers normally trim it; being lenient here costs nothing).
func BearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

type userKey struct{}

// WithUser returns a context carrying the authenticated user.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// FromContext returns the authenticated user, if one is attached.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}
