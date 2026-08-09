package security

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	apperrors "github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/errors"
)

// AccessClaims is the payload of the access token.
//
// §9: the token carries no company. There is no global company context on the
// server or in the client, which is what lets two browser tabs work on two
// companies at the same time without interfering.
type AccessClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type TokenIssuer struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewTokenIssuer(cfg config.JWTConfig) *TokenIssuer {
	return &TokenIssuer{
		secret:     []byte(cfg.Secret),
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// AccessTTL exposes the configured lifetime so the login response can tell the
// client when to refresh.
func (t *TokenIssuer) AccessTTL() time.Duration  { return t.accessTTL }
func (t *TokenIssuer) RefreshTTL() time.Duration { return t.refreshTTL }

// IssueAccessToken returns a signed access token and its jti.
func (t *TokenIssuer) IssueAccessToken(userID int64, username string) (string, string, error) {
	jti := uuid.NewString()
	now := t.now()

	claims := AccessClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ID:        jti,
			Issuer:    t.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.accessTTL)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", "", apperrors.ErrInternal.WithCause(err)
	}
	return signed, jti, nil
}

// ParseAccessToken validates signature, expiry and issuer, returning the
// principal. A malformed or expired token yields 401, never 403.
func (t *TokenIssuer) ParseAccessToken(token string) (Principal, error) {
	claims := &AccessClaims{}

	_, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return t.secret, nil
	}, jwt.WithIssuer(t.issuer), jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Principal{}, apperrors.ErrTokenExpired
		}
		return Principal{}, apperrors.ErrUnauthorized.WithCause(err)
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID == 0 {
		return Principal{}, apperrors.ErrUnauthorized
	}

	return Principal{UserID: userID, Username: claims.Username, TokenID: claims.ID}, nil
}
