package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrSealed is returned when the stored key (or an encrypted secret) cannot be
// decrypted with the configured master key — e.g. the env var changed or the
// row was tampered with.
var ErrSealed = errors.New("settings: cannot decrypt with configured master key")

// gcmNonceSize is the standard 12-byte AES-GCM nonce length; it is also the
// layout used by encryptGmail/decryptGmail, which append the nonce directly
// after the ciphertext.
const gcmNonceSize = 12

// Encrypter wraps the 32-byte AES key this package uses. It encrypts and
// decrypts byte slices with AES-256-GCM; every Encrypt call draws a fresh
// random 12-byte nonce, and Decrypt takes that nonce back alongside the
// ciphertext.
type Encrypter struct {
	key [32]byte
}

// NewEncrypter derives the 32-byte AES-256 key from masterKey with SHA-256, so
// the master key may be any length (an env var value, a passphrase, a hex
// string, …). It never errors: a nil or empty master key derives a
// deterministic key, which callers can still use — although a real deployment
// must pass a non-empty secret.
func NewEncrypter(masterKey []byte) *Encrypter {
	return &Encrypter{key: sha256.Sum256(masterKey)}
}

// Encrypt seals plaintext with AES-256-GCM under a fresh random 12-byte nonce
// and returns the ciphertext and that nonce. The two must be stored together
// and passed back to Decrypt in the same pairing.
func (e *Encrypter) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, nil, fmt.Errorf("settings: new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: new gcm: %w", err)
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("settings: read random nonce: %w", err)
	}
	return aead.Seal(nil, nonce, plaintext, nil), nonce, nil
}

// Decrypt opens ciphertext sealed by Encrypt with the same Encrypter (same
// key) and the nonce produced by that call.
func (e *Encrypter) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, fmt.Errorf("settings: new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("settings: new gcm: %w", err)
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("%w: nonce size %d, want %d", ErrSealed, len(nonce), aead.NonceSize())
	}
	out, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSealed, err)
	}
	return out, nil
}
