// Package server exposes the health endpoint. It answers one question: does this need a
// human? A dead session does; a failed pass that will retry does not.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/PlatanosVerdes/wallapop-reactivator/internal/reactivate"
	"github.com/PlatanosVerdes/wallapop-reactivator/internal/session"
)

type Health struct {
	Version    string
	DataDir    string
	Store      *session.Store
	WarnBefore time.Duration
	NextRun    func() time.Time
}

type payload struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Session string `json:"session"`
	// The access token lasts minutes and is minted on every pass, so what is worth
	// reporting is how long the session can keep minting them.
	RenewableDays *float64           `json:"renewable_days_left,omitempty"`
	NextRun       string             `json:"next_run,omitempty"`
	LastRun       *reactivate.Result `json:"last_run,omitempty"`
}

func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// Always 200 while the process is alive. A session that needs a human is not a
		// service that stopped answering, and the blackbox probe reads any other code as
		// exactly that: the alert for the session comes from wallapop_last_run_status.
		body := payload{Status: "ok", Version: h.Version, Session: "ok"}
		down := func(why string) {
			body.Session = why
			body.Status = "down"
		}

		sess, err := h.Store.Load()
		switch {
		case err != nil:
			down(err.Error())
		case sess.CookieValue == "":
			down("no session cookie stored")
		default:
			if left, ok := sess.Renewable(); ok {
				days := left.Hours() / 24
				body.RenewableDays = &days
				switch {
				case left <= 0:
					down("the session cookie has expired")
				case left <= h.WarnBefore:
					body.Session = "expiring"
					body.Status = "warn"
				}
			}
		}

		if res, ok := reactivate.LoadResult(h.DataDir); ok {
			body.LastRun = &res
			switch {
			case res.NeedsHuman:
				down("the last pass was rejected by Wallapop")
			case res.Error != "" && body.Status == "ok":
				body.Status = "warn"
			}
		}
		if h.NextRun != nil {
			if next := h.NextRun(); !next.IsZero() {
				body.NextRun = next.Format(time.RFC3339)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(body)
	})
	return mux
}
