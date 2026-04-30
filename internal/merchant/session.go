package merchant

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const DashboardSessionCookieName = "paygate_dashboard_session"

type sessionClaims struct {
	UserID     string           `json:"user_id"`
	MerchantID string           `json:"merchant_id"`
	Email      string           `json:"email"`
	Role       MerchantUserRole `json:"role"`
	ExpiresAt  int64            `json:"exp"`
	IssuedAt   int64            `json:"iat"`
}

type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &SessionManager{secret: []byte(secret), ttl: ttl}
}

func (m *SessionManager) Issue(user MerchantUser) (string, error) {
	if m == nil || len(m.secret) == 0 {
		return "", ErrDashboardSession
	}
	now := time.Now().UTC()
	claims := sessionClaims{
		UserID:     user.ID,
		MerchantID: user.MerchantID,
		Email:      user.Email,
		Role:       user.Role,
		ExpiresAt:  now.Add(m.ttl).Unix(),
		IssuedAt:   now.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := m.sign(encodedPayload)
	return encodedPayload + "." + sig, nil
}

func (m *SessionManager) Parse(token string) (sessionClaims, error) {
	if m == nil || len(m.secret) == 0 {
		return sessionClaims{}, ErrDashboardSession
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return sessionClaims{}, ErrDashboardSession
	}
	if !hmac.Equal([]byte(parts[1]), []byte(m.sign(parts[0]))) {
		return sessionClaims{}, ErrDashboardSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, ErrDashboardSession
	}
	var claims sessionClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return sessionClaims{}, ErrDashboardSession
	}
	if claims.UserID == "" || claims.MerchantID == "" || claims.ExpiresAt == 0 {
		return sessionClaims{}, ErrDashboardSession
	}
	if time.Now().UTC().Unix() >= claims.ExpiresAt {
		return sessionClaims{}, errors.Join(ErrDashboardSession, errors.New("session expired"))
	}
	return claims, nil
}

func (m *SessionManager) TTL() time.Duration {
	if m == nil || m.ttl <= 0 {
		return time.Hour
	}
	return m.ttl
}

func (m *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
