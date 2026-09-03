# Deploy

Built from this repo by [rpi-services](https://github.com/PlatanosVerdes/rpi-services),
pinned to the CalVer tag that `auto-tag.yml` creates on every push to main.

Entry for its `docker-compose.yml`:

```yaml
  # --- Wallapop: reactivar anuncios caducados una vez al dia ---
  wallapop-reactivator:
    build:
      context: https://github.com/PlatanosVerdes/wallapop-reactivator.git#${WALLAPOP_VERSION}
      args:
        VERSION: ${WALLAPOP_VERSION}
    container_name: wallapop-reactivator
    labels:
      prometheus.probe: "http://wallapop-reactivator:8000/healthz"
    restart: unless-stopped
    profiles: [wallapop, all]
    mem_limit: 64m
    memswap_limit: 64m
    dns:
      - 8.8.8.8
      - 1.1.1.1
    networks:
      - media-network
    environment:
      TZ: ${TZ:-Europe/Madrid}
      WALLA_TELEGRAM_TOKEN: ${WALLA_TELEGRAM_TOKEN}
      WALLA_TELEGRAM_CHAT: ${WALLA_TELEGRAM_CHAT}
    volumes:
      # The session must persist: it is imported by hand and renews itself in place.
      - ${APP_CONFIG_PATH}/wallapop-reactivator:/data
```

The session is the only manual step, and only when the cookie finally expires:

```bash
docker exec wallapop-reactivator wallapop session import --cookie '<the __Secure-next-auth.session-token value>'
docker exec wallapop-reactivator wallapop session show
```

The import renews once before reporting success, so it doubles as the check.

A dry pass without waiting for the ticker:

```bash
docker exec wallapop-reactivator wallapop run --dry-run
```
