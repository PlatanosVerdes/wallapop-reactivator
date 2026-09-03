// Package session keeps the browser session on disk. It is the one thing here that
// cannot be regenerated without a human: it is imported once from the browser and read
// on every run.
package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrMissing = errors.New("session: no session imported yet")

// DefaultCookieName is the NextAuth session cookie. Its value is encrypted and holds the
// refresh token, which is why the browser itself never sees one.
const DefaultCookieName = "__Secure-next-auth.session-token"

type Session struct {
	// CookieValue is the durable half: access tokens are minted from it.
	CookieName  string `json:"cookie_name"`
	CookieValue string `json:"cookie_value"`
	AccessToken string `json:"access_token,omitempty"`

	DeviceID string `json:"device_id,omitempty"`
	UserID   string `json:"user_id,omitempty"`

	ImportedAt time.Time `json:"imported_at"`
	RenewedAt  time.Time `json:"renewed_at,omitempty"`
	// Expires is what the session endpoint reports: the deadline past which a human has
	// to import a new cookie.
	Expires time.Time `json:"expires,omitempty"`
}

// Store serialises reads and writes so the HTTP handler and the ticker can share one
// session file.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  *Session
}

func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, "session.json")}
}

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.path, err)
	}
	if strings.TrimSpace(sess.CookieValue) == "" {
		return nil, fmt.Errorf("%s carries no session cookie", s.path)
	}
	if sess.CookieName == "" {
		sess.CookieName = DefaultCookieName
	}
	s.cur = &sess
	return &sess, nil
}

func (s *Store) Save(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.cur = sess
	return nil
}

// AccessToken, SessionCookie and Update are what wallapop.Client renews through.
func (s *Store) AccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cur == nil {
		return ""
	}
	return s.cur.AccessToken
}

func (s *Store) SessionCookie() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cur == nil {
		return DefaultCookieName, ""
	}
	return s.cur.CookieName, s.cur.CookieValue
}

// Update stores what a renewal returned. The cookie rolls on every read, so a new value
// replaces the stored one and the session stays alive as long as passes keep happening.
func (s *Store) Update(access, cookie string, expires time.Time) error {
	s.mu.RLock()
	cur := s.cur
	s.mu.RUnlock()
	if cur == nil {
		return ErrMissing
	}

	next := *cur
	next.AccessToken = access
	next.RenewedAt = time.Now()
	if cookie != "" {
		next.CookieValue = cookie
	}
	if !expires.IsZero() {
		next.Expires = expires
	}
	next.fillFromClaims()
	return s.Save(&next)
}

// AccessSpent says whether renewing first would save a rejected request. An unreadable
// token is left to the API to judge.
func (s *Store) AccessSpent() bool {
	s.mu.RLock()
	cur := s.cur
	s.mu.RUnlock()
	if cur == nil || cur.AccessToken == "" {
		return true
	}
	left, ok := cur.TimeLeft()
	return ok && left <= time.Minute
}

func (s *Store) Current() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Claims are the fields of the access token this service reads. The token is not ours to
// validate, only to read: device_id is echoed back as the x-deviceid header, and exp says
// whether renewing first is worth it.
type Claims struct {
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
	AuthTime int64  `json:"auth_time"`
	Sub      string `json:"sub"`
	Iss      string `json:"iss"`
	Azp      string `json:"azp"`
	DeviceID string `json:"device_id"`
}

func (s *Session) Claims() (Claims, bool) {
	parts := strings.Split(s.AccessToken, ".")
	if len(parts) < 2 {
		return Claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, false
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, false
	}
	return claims, claims.Exp != 0
}

func (s *Session) ExpiresAt() (time.Time, bool) {
	claims, ok := s.Claims()
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

func (s *Session) TimeLeft() (time.Duration, bool) {
	exp, ok := s.ExpiresAt()
	if !ok {
		return 0, false
	}
	return time.Until(exp), true
}

// Renewable is how long the session can keep minting access tokens unattended.
func (s *Session) Renewable() (time.Duration, bool) {
	if s.Expires.IsZero() {
		return 0, false
	}
	return time.Until(s.Expires), true
}

// Device returns the device id to send with every call: the stored one wins, and the
// token claim covers a session whose cookie was imported on its own.
func (s *Session) Device() string {
	if s.DeviceID != "" {
		return s.DeviceID
	}
	if claims, ok := s.Claims(); ok {
		return claims.DeviceID
	}
	return ""
}

// New builds a session from the NextAuth cookie, which is all the browser can give.
func New(cookieValue string) *Session {
	return &Session{
		CookieName:  DefaultCookieName,
		CookieValue: strings.TrimSpace(cookieValue),
		ImportedAt:  time.Now(),
	}
}

// Parse accepts the cookie value on its own, a name=value pair copied from DevTools, or a
// JSON object naming it.
func Parse(input string) (*Session, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("session: nothing to import")
	}

	if strings.HasPrefix(input, "{") {
		var raw struct {
			CookieValue  string `json:"cookie_value"`
			SessionToken string `json:"session_token"`
			NextAuth     string `json:"__Secure-next-auth.session-token"`
		}
		if err := json.Unmarshal([]byte(input), &raw); err != nil {
			return nil, fmt.Errorf("session: that JSON did not parse: %w", err)
		}
		value := firstOf(raw.CookieValue, raw.SessionToken, raw.NextAuth)
		if value == "" {
			return nil, errors.New("session: the JSON names no session cookie")
		}
		return New(value), nil
	}

	sess := New(input)
	if name, value, found := strings.Cut(input, "="); found && strings.HasPrefix(name, "__") {
		sess.CookieName = strings.TrimSpace(name)
		sess.CookieValue = strings.TrimSpace(value)
	}
	// A NextAuth cookie is an encrypted JWE: five dot-separated parts.
	if strings.Count(sess.CookieValue, ".") != 4 {
		return nil, errors.New("session: that does not look like a NextAuth session cookie (expected five dot-separated parts)")
	}
	return sess, nil
}

func (s *Session) fillFromClaims() {
	claims, ok := s.Claims()
	if !ok {
		return
	}
	if s.DeviceID == "" {
		s.DeviceID = claims.DeviceID
	}
	if s.UserID == "" {
		s.UserID = claims.Sub
	}
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
