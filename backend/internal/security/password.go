package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"

	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
)

// argon2id parameters (§C4). Tuned for ~50 ms on a typical application server.
const (
	argonTime    = 2
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// PasswordHasher hashes and verifies user passwords.
type PasswordHasher struct {
	minLength int
}

func NewPasswordHasher(minLength int) *PasswordHasher {
	if minLength <= 0 {
		minLength = 12
	}
	return &PasswordHasher{minLength: minLength}
}

// Hash produces a PHC-formatted argon2id hash:
//
//	$argon2id$v=19$m=65536,t=2,p=4$<salt>$<key>
func (h *PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify reports whether the password matches the stored hash. It compares in
// constant time and never distinguishes "wrong password" from "malformed hash"
// to the caller.
func (h *PasswordHasher) Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}

	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidatePolicy enforces the password policy of §C4: minimum length plus a
// mix of character classes.
func (h *PasswordHasher) ValidatePolicy(password string) error {
	var details []apperrors.Detail

	if len([]rune(password)) < h.minLength {
		details = append(details, apperrors.Detail{
			Field:   "password",
			Message: fmt.Sprintf("must be at least %d characters long", h.minLength),
		})
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		details = append(details, apperrors.Detail{
			Field:   "password",
			Message: "must contain upper case, lower case, a digit and a special character",
		})
	}

	if len(details) > 0 {
		return apperrors.ErrPasswordPolicy.WithDetails(details...)
	}
	return nil
}

// HashToken derives the storage form of a refresh token. Refresh tokens are
// high-entropy random strings, so a single SHA-256 pass is sufficient and
// keeps token rotation cheap — unlike user passwords, which need argon2id.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewOpaqueToken returns a cryptographically random refresh token.
func NewOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
