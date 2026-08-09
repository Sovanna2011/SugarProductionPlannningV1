package security

import (
	"strings"
	"testing"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
)

func TestHashAndVerify(t *testing.T) {
	hasher := NewPasswordHasher(12)

	hash, err := hasher.Hash("Correct#Horse2026")
	if err != nil {
		t.Fatalf("hashing failed: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected an argon2id PHC string, got %q", hash)
	}
	if !hasher.Verify("Correct#Horse2026", hash) {
		t.Error("the correct password must verify")
	}
	if hasher.Verify("correct#horse2026", hash) {
		t.Error("a wrong password must not verify")
	}
}

// TestHashIsSalted checks that two hashes of the same password differ, so a
// leaked table cannot be attacked with a single precomputed dictionary.
func TestHashIsSalted(t *testing.T) {
	hasher := NewPasswordHasher(12)

	first, err := hasher.Hash("Correct#Horse2026")
	if err != nil {
		t.Fatal(err)
	}
	second, err := hasher.Hash("Correct#Horse2026")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("two hashes of the same password must differ")
	}
}

// TestVerifyRejectsMalformedHash makes sure a corrupted stored value fails
// closed rather than panicking or accidentally matching.
func TestVerifyRejectsMalformedHash(t *testing.T) {
	hasher := NewPasswordHasher(12)

	for _, bad := range []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=2,p=4$onlyonepart",
		"$bcrypt$v=19$m=65536,t=2,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=99$m=65536,t=2,p=4$c2FsdA$aGFzaA",
	} {
		if hasher.Verify("anything", bad) {
			t.Errorf("a malformed hash must never verify: %q", bad)
		}
	}
}

func TestPasswordPolicy(t *testing.T) {
	hasher := NewPasswordHasher(12)

	valid := []string{"Correct#Horse2026", "Sugar!Planning99", "aB3$aB3$aB3$"}
	for _, password := range valid {
		if err := hasher.ValidatePolicy(password); err != nil {
			t.Errorf("%q should satisfy the policy: %v", password, err)
		}
	}

	invalid := map[string]string{
		"too short":       "Ab3$xY9",
		"no upper case":   "sugar#planning2026",
		"no lower case":   "SUGAR#PLANNING2026",
		"no digit":        "Sugar#PlanningTest",
		"no special char": "SugarPlanning2026",
	}
	for reason, password := range invalid {
		if err := hasher.ValidatePolicy(password); err == nil {
			t.Errorf("%q should be rejected (%s)", password, reason)
		}
	}
}

// TestTokenRoundTrip checks that an access token carries the principal — and,
// per §9, that it does not carry a company.
func TestTokenRoundTrip(t *testing.T) {
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:    "test-secret-that-is-long-enough-for-hs256",
		Issuer:    "sugar-planning-test",
		AccessTTL: 15 * 60 * 1e9,
	})

	token, jti, err := issuer.IssueAccessToken(42, "planner")
	if err != nil {
		t.Fatalf("issuing failed: %v", err)
	}
	if jti == "" {
		t.Error("the token must carry a jti")
	}

	principal, err := issuer.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}
	if principal.UserID != 42 || principal.Username != "planner" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestTokenRejectsForeignSignature(t *testing.T) {
	mine := NewTokenIssuer(config.JWTConfig{
		Secret: "test-secret-that-is-long-enough-for-hs256", Issuer: "sugar", AccessTTL: 15 * 60 * 1e9,
	})
	theirs := NewTokenIssuer(config.JWTConfig{
		Secret: "a-completely-different-signing-key-value", Issuer: "sugar", AccessTTL: 15 * 60 * 1e9,
	})

	token, _, err := theirs.IssueAccessToken(1, "attacker")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mine.ParseAccessToken(token); err == nil {
		t.Error("a token signed with another key must be rejected")
	}
}

func TestHashTokenIsDeterministic(t *testing.T) {
	token, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(token) != HashToken(token) {
		t.Error("the refresh-token hash must be stable so a lookup can find it")
	}
	other, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(token) == HashToken(other) {
		t.Error("two random tokens must not collide")
	}
}
