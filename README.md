# wallapop-reactivator

Keeps my Wallapop catalogue on the market. A listing expires after a while and goes quiet
until somebody presses **Reactivar** on it; this service does that pass once a day, and
says nothing unless something needs a human.

Runs on the Raspberry from [rpi-services](https://github.com/PlatanosVerdes/rpi-services),
same conventions as the rest: Go with no dependencies, CalVer tag on every push to main,
`/healthz` for the blackbox probe.

## The API

Wallapop's [Connect API](https://developers.wallapop.com) is open to PRO sellers and
integrators with a registered OAuth client, and documents no reactivate call, so this uses
the same private API the web app does. Two endpoints, both captured from the browser:

| Call | What for |
| :--- | :--- |
| `GET /api/v3/user/items` | The catalogue. The pagination cursor comes back in the `X-NextPage` response header, not in the body |
| `PUT /api/v3/items/{id}/reactivate` | Presses the button. Answers `204 No Content` |

Requests carry the bearer plus the device headers a browser sends (`x-deviceid`,
`x-appversion`, `deviceos`). There is no request signing: the older `X-Signature` scheme
(HMAC-SHA256 over method, path and timestamp) is gone from the web app, and lives on here
behind `WALLA_SIGN_SCHEME` for the day it returns.

## The session

This is the part that needs care. The access token is a Keycloak token
(`accounts.wallapop.com`, realm `wallapop-internal`, client `web`) and **lasts five
minutes**: `exp - iat` is exactly 300 seconds. But there is no refresh token to store:
the web app is a NextAuth app, and the refresh token is encrypted inside the
`__Secure-next-auth.session-token` cookie, which is `httpOnly` and only its own server can
read. That is why renewing a session produces no traffic to `accounts.wallapop.com` at
all.

So the way to get a token is the same one the page uses, `getSession()`:

| Call | What for |
| :--- | :--- |
| `GET es.wallapop.com/api/auth/session` | Send the session cookie, get `{"token": "<access token>", "expires": "…"}` back. Their server does the Keycloak refresh |

The cookie rolls on every read, so a renewal stores the new value back and the session
outlives any single token: a daily pass keeps it alive indefinitely. A revoked cookie is
answered with an empty session and a 200, not an error, which is treated as the one case
that needs a human.

When an API call is rejected, the `x-wallapop-unauthorized` response header says which
half expired: `ACCESS_TOKEN_EXPIRED` is renewed and retried on the spot,
`REFRESH_TOKEN_EXPIRED` wakes a human. That is the difference between a quiet retry and a
Telegram message.

Automating this is against Wallapop's terms of service. It is my own account and my own
ten listings, so the practical risk is being flagged as a bot: hence one pass a day and a
random 20-90s pause between listings.

## Commands

```bash
wallapop run --dry-run   # list what would be reactivated, touch nothing
wallapop run             # one pass
wallapop serve           # daily pass + /healthz on :8000
wallapop session import --cookie '<value>'   # store the browser session cookie
wallapop session show    # what is stored and whether it can renew itself
wallapop session refresh # renew now, which is how you check it works
```

## Importing a session

In DevTools, **Application → Cookies → `https://es.wallapop.com`**, copy the value of
`__Secure-next-auth.session-token`:

```bash
wallapop session import --cookie '<the cookie value>'
```

The import renews once before reporting success, so "stored" and "works" are the same
thing. `session refresh` repeats that check at any time, and `session show` reports how
long the cookie has left.

## Configuration

| Variable | Default | What it does |
| :--- | :--- | :--- |
| `WALLA_DATA_DIR` | `./data` | Where the session and the last run are kept |
| `WALLA_INTERVAL` | `24h` | Time between passes in `serve`. Under a minute is refused |
| `WALLA_RETRY_EVERY` | `15m` | How soon a failed pass is tried again. Under a minute is refused |
| `WALLA_PORT` | `8000` | Port for `/healthz` |
| `WALLA_MIN_PAUSE` / `WALLA_MAX_PAUSE` | `20s` / `90s` | Random pause between listings |
| `WALLA_MAX_PER_RUN` | `25` | Ceiling on listings touched in one pass |
| `WALLA_WARN_BEFORE` | `72h` | How early the end of the session is announced |
| `WALLA_PUSHGATEWAY` | – | Pushgateway base URL. Empty means report nothing, which is right off the Pi |
| `WALLA_DEVICE_ID` | from the `device_id` claim | Sent as `x-deviceid` |
| `WALLA_APP_VERSION` | `826680` | Sent as `x-appversion` |
| `WALLA_PATH_ITEMS` | `/api/v3/user/items` | Catalogue endpoint |
| `WALLA_PATH_REACTIVATE` | `/api/v3/items/%s/reactivate` | Reactivate endpoint, `%s` is the item id |
| `WALLA_REACTIVATE_METHOD` | `PUT` | Verb for the reactivate call |
| `WALLA_WEB_URL` | `https://es.wallapop.com` | The site, which is where sessions are renewed |
| `WALLA_SIGN_SCHEME` | `none` | `none`, `pipe` or `legacy`, if they ever sign requests again |
| `WALLA_LOG_JSON` | – | `1` for JSON logs, which is what Vector collects |

The endpoint variables exist so a change on Wallapop's side is a redeploy and not a
rebuild.

## Health

`/healthz` answers **200 whenever the process is alive**, and the body carries the state:

```json
{
  "status": "ok",
  "session": "ok",
  "renewable_days_left": 27.4,
  "next_run": "2026-09-04T09:00:00+02:00",
  "last_run": { "catalogue": 10, "expired": 6, "reactivated": ["Apple Watch SE 2 44mm"] }
}
```

`status` is `down` when there is no session or the cookie has expired, `warn` when it ends
within `WALLA_WARN_BEFORE` or the last pass failed, and `ok` otherwise.

It never answers 503 for a session problem, and that is deliberate: a blackbox probe reads
any other code as "the service stopped answering", which would raise the wrong alert with
the wrong words for a service that is answering perfectly well and only needs a new cookie.
Liveness is the probe's question; the session is the metric's.

## Alerting

The service sends no messages of its own. It pushes gauges to the Pushgateway after every
pass and the alert rules decide what deserves attention, which is what keeps every alert in
one place and in one format:

| Metric | What it says |
| :--- | :--- |
| `wallapop_last_run_status` | 0 fine, 1 failed and a retry may fix it, 2 needs a human |
| `wallapop_last_run_timestamp` | Unix time of the last pass, so a service that stopped running is visible |
| `wallapop_session_days_remaining` | Days of unattended runway left, -1 when not known yet |
| `wallapop_expired_listings` | Listings found expired in the last pass |
| `wallapop_reactivated_listings` | Listings reactivated in the last pass |

A dry run reports nothing, so trying things out does not move the graphs.

A failed pass is retried every `WALLA_RETRY_EVERY` instead of waiting for the next daily
tick, so a session imported by hand takes effect in minutes and the alert clears itself.
