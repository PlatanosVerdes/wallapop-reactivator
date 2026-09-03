package wallapop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Renewing a session never touches Keycloak, which is why a browser doing it produces no
// traffic to accounts.wallapop.com. The web app is a NextAuth app: the refresh token is
// encrypted inside the session cookie and only its own server can read it, so the way to
// get a fresh access token is to ask that server for the session, exactly as the page
// does through getSession().
const (
	DefaultWebURL = "https://es.wallapop.com"
	PathSession   = "/api/auth/session"
)

const (
	// headerUnauthorized says which half expired, which is the difference between
	// retrying and waking a human.
	headerUnauthorized = "x-wallapop-unauthorized"

	reasonAccessExpired  = "ACCESS_TOKEN_EXPIRED"
	reasonRefreshExpired = "REFRESH_TOKEN_EXPIRED"
)

// TokenSource is the session the client reads and renews. session.Store implements it.
type TokenSource interface {
	AccessToken() string
	SessionCookie() (name, value string)
	Update(access, cookie string, expires time.Time) error
}

// RenewSession mints a fresh access token from the session cookie. The cookie rolls on
// every read, so a rotated one is stored back and the session outlives any single token.
func (c *Client) RenewSession(ctx context.Context) error {
	name, value := c.Session.SessionCookie()
	if value == "" {
		return fmt.Errorf("%w: no session cookie stored", ErrUnauthorized)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.WebURL+PathSession, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "es,en-US;q=0.9")
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Referer", c.WebURL+"/")
	req.Header.Set("Cookie", name+"="+value)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("renewing the session: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: %s answered %d: %s", ErrUnauthorized, PathSession, resp.StatusCode, snippet(raw))
	}

	var answer struct {
		Token   string `json:"token"`
		Expires string `json:"expires"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return fmt.Errorf("decoding the session answer: %w (body: %s)", err, snippet(raw))
	}
	// An expired or revoked cookie is answered with an empty session, not an error.
	if answer.Token == "" {
		return fmt.Errorf("%w: the session endpoint returned no token (%s)", ErrUnauthorized, snippet(raw))
	}

	expires, err := time.Parse(time.RFC3339, answer.Expires)
	if err != nil {
		expires = time.Time{}
	}
	return c.Session.Update(answer.Token, rolledCookie(resp.Cookies(), name), expires)
}

func rolledCookie(cookies []*http.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

// classify turns a rejected response into the two cases that matter.
func classify(status int, header http.Header, path string, body []byte) error {
	reason := header.Get(headerUnauthorized)
	switch {
	case reason == reasonRefreshExpired:
		return fmt.Errorf("%w: the session can no longer be renewed", ErrUnauthorized)
	case reason == reasonAccessExpired, status == http.StatusUnauthorized:
		return fmt.Errorf("%w (%d on %s)", ErrAccessExpired, status, path)
	case status == http.StatusForbidden:
		return fmt.Errorf("%w (403 on %s): %s", ErrUnauthorized, path, snippet(body))
	}
	return errors.New("classify called on a response that was not a rejection")
}
