package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultIssuer   = "spindle-edge"
	defaultAudience = "edge-terminal"
)

var (
	ErrTokenInvalid = errors.New("token is invalid")
	ErrTokenExpired = errors.New("token is expired")
)

type JWTManager struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
	now      func() time.Time
}

type Claims struct {
	Issuer             string `json:"iss"`
	Audience           string `json:"aud"`
	Subject            string `json:"sub"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	PermissionsVersion int64  `json:"permissions_version"`
	IssuedAt           int64  `json:"iat"`
	NotBefore          int64  `json:"nbf"`
	ExpiresAt          int64  `json:"exp"`
	JTI                string `json:"jti"`
}

type UserTokenSubject struct {
	ID                 uint
	Username           string
	Role               string
	PermissionsVersion int64
}

func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &JWTManager{
		secret:   []byte(secret),
		issuer:   defaultIssuer,
		audience: defaultAudience,
		ttl:      ttl,
		now:      time.Now,
	}
}

func (m *JWTManager) TTL() time.Duration {
	return m.ttl
}

func (m *JWTManager) Sign(subject UserTokenSubject) (string, Claims, error) {
	now := m.now().Unix()
	jti, err := GenerateOpaqueToken(16)
	if err != nil {
		return "", Claims{}, err
	}
	claims := Claims{
		Issuer:             m.issuer,
		Audience:           m.audience,
		Subject:            strconv.FormatUint(uint64(subject.ID), 10),
		Username:           subject.Username,
		Role:               subject.Role,
		PermissionsVersion: subject.PermissionsVersion,
		IssuedAt:           now,
		NotBefore:          now,
		ExpiresAt:          m.now().Add(m.ttl).Unix(),
		JTI:                jti,
	}
	token, err := m.encode(claims)
	return token, claims, err
}

func (m *JWTManager) Validate(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrTokenInvalid
	}
	signingInput := parts[0] + "." + parts[1]
	expected := signHS256([]byte(signingInput), m.secret)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, ErrTokenInvalid
	}

	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerRaw, &header) != nil || header.Algorithm != "HS256" {
		return Claims{}, ErrTokenInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrTokenInvalid
	}
	if claims.Issuer != m.issuer || claims.Audience != m.audience || claims.Subject == "" {
		return Claims{}, ErrTokenInvalid
	}
	now := m.now().Unix()
	if claims.ExpiresAt <= now {
		return Claims{}, ErrTokenExpired
	}
	if claims.NotBefore > now {
		return Claims{}, ErrTokenInvalid
	}
	return claims, nil
}

func (m *JWTManager) encode(claims Claims) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	return fmt.Sprintf("%s.%s", signingInput, signHS256([]byte(signingInput), m.secret)), nil
}

func signHS256(input []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(input)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
