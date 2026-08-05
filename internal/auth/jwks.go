// JWKS verification: validates ES256/RS256-signed tokens against the asymmetric
// public keys a provider publishes at its jwks.json Discovery URL. Parsing is
// stdlib-only (no JWKS dependency) and the key set is cached with a short TTL,
// refreshing when a token's `kid` is unknown (key rotation).

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwksAlgs are the asymmetric signing methods accepted in JWKS mode.
var jwksAlgs = []string{jwt.SigningMethodES256.Alg(), jwt.SigningMethodRS256.Alg()}

// verifyJWKS validates tokenString against the provider's asymmetric JWKS keys.
func (v *Verifier) verifyJWKS(tokenString string) (User, error) {
	claims := &Claims{}
	opts := []jwt.ParserOption{
		jwt.WithValidMethods(jwksAlgs),
		jwt.WithExpirationRequired(),
	}
	if v.issuer != "" {
		opts = append(opts, jwt.WithIssuer(v.issuer))
	}
	opts = append(opts, jwt.WithAudience(v.audience))

	tok, err := jwt.ParseWithClaims(tokenString, claims, v.jwksKeyfunc, opts...)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return User{}, ErrExpired
		}
		return User{}, fmt.Errorf("%w: %v", ErrBadToken, err)
	}
	if tok == nil || !tok.Valid {
		return User{}, ErrBadToken
	}
	return userFromClaims(claims), nil
}

// jwksKeyfunc selects the verification key by the token's "kid" header.
func (v *Verifier) jwksKeyfunc(t *jwt.Token) (any, error) {
	switch t.Method.(type) {
	case *jwt.SigningMethodECDSA, *jwt.SigningMethodRSA:
	default:
		return nil, fmt.Errorf("%w: unsupported signing method %s", ErrBadToken, t.Method.Alg())
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("%w: token has no kid", ErrBadToken)
	}
	return v.keyForID(kid)
}

// keyForID returns the cached public key for kid, refreshing the key set when
// the kid is unknown (providers rotate keys).
func (v *Verifier) keyForID(kid string) (any, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	if time.Since(v.fetched) < v.jwksTTL {
		return nil, fmt.Errorf("%w: unknown kid %s", ErrBadToken, kid)
	}
	keys, err := v.fetchJWKS()
	if err != nil {
		return nil, err
	}
	v.keys = keys
	v.fetched = time.Now()
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("%w: unknown kid %s", ErrBadToken, kid)
}

// fetchJWKS downloads and parses the provider's public key set.
func (v *Verifier) fetchJWKS() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build jwks request: %v", ErrBadToken, err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch jwks: %v", ErrBadToken, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: jwks http %d", ErrBadToken, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read jwks: %v", ErrBadToken, err)
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse jwks: %v", ErrBadToken, err)
	}
	keys := make(map[string]any)
	for _, k := range doc.Keys {
		key, err := parseJWKSKey(k.Kty, k.Crv, k.X, k.Y, k.N, k.E)
		if err != nil {
			continue // skip unsupported keys; fail only if none are usable
		}
		keys[k.Kid] = key
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: jwks contains no usable keys", ErrBadToken)
	}
	return keys, nil
}

// parseJWKSKey builds a crypto public key from a JWK entry (stdlib only).
func parseJWKSKey(kty, crv, x, y, n, e string) (any, error) {
	switch kty {
	case "EC":
		curve, err := curveForCRV(crv)
		if err != nil {
			return nil, err
		}
		xb, err := decodeB64(x)
		if err != nil {
			return nil, err
		}
		yb, err := decodeB64(y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		}, nil
	case "RSA":
		nb, err := decodeB64(n)
		if err != nil {
			return nil, err
		}
		eb, err := decodeB64(e)
		if err != nil {
			return nil, err
		}
		var ei int
		for _, b := range eb {
			ei = ei<<8 | int(b)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: ei}, nil
	}
	return nil, fmt.Errorf("unsupported kty %s", kty)
}

// curveForCRV maps a JWK curve name to a crypto curve.
func curveForCRV(crv string) (elliptic.Curve, error) {
	switch crv {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	}
	return nil, fmt.Errorf("unsupported curve %s", crv)
}

func decodeB64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
