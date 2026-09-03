package wallapop

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// SignScheme selects how the X-Signature payload is built. The web app no longer signs
// anything (a captured 2026 request carries only the bearer and the device headers), so
// the default is none and the two older layouts stay here for the day it comes back.
type SignScheme string

const (
	SchemeNone   SignScheme = "none"
	SchemePipe   SignScheme = "pipe"
	SchemeLegacy SignScheme = "legacy"
)

func ValidScheme(s SignScheme) bool {
	if s == SchemeNone {
		return true
	}
	_, ok := signKeys[s]
	return ok
}

// Keys shipped inside the client, recovered by rmonvfer/wallapop_secret. Each one is
// base64 of the actual HMAC key.
var signKeys = map[SignScheme]string{
	SchemePipe:   "Tm93IHRoYXQgeW91J3ZlIGZvdW5kIHRoaXMsIGFyZSB5b3UgcmVhZHkgdG8gam9pbiB1cz8gam9ic0B3YWxsYXBvcC5jb20=",
	SchemeLegacy: "UTI5dVozSmhkSE1zSUhsdmRTZDJaU0JtYjNWdVpDQnBkQ0VnUVhKbElIbHZkU0J5WldGa2VTQjBieUJxYjJsdUlIVnpQeUJxYjJKelFIZGhiR3hoY0c5d0xtTnZiUT09",
}

func Schemes() []SignScheme { return []SignScheme{SchemePipe, SchemeLegacy} }

// Sign returns the X-Signature value for a request. path is the request path with no
// host and no query string; ts is the same value sent in the timestamp header.
func Sign(scheme SignScheme, method, path string, ts int64) (string, error) {
	encoded, ok := signKeys[scheme]
	if !ok {
		return "", fmt.Errorf("unknown signature scheme %q", scheme)
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decoding %s key: %w", scheme, err)
	}

	method = strings.ToUpper(method)
	stamp := strconv.FormatInt(ts, 10)

	var payload string
	switch scheme {
	case SchemeLegacy:
		payload = path + "+#+" + method + "+#+" + stamp + "+#+"
	default:
		payload = method + "|" + path + "|" + stamp + "|"
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// MatchScheme reports which scheme reproduces a signature captured from the browser.
func MatchScheme(method, path string, ts int64, want string) (SignScheme, bool) {
	for _, s := range Schemes() {
		got, err := Sign(s, method, path, ts)
		if err == nil && hmac.Equal([]byte(got), []byte(want)) {
			return s, true
		}
	}
	return "", false
}
