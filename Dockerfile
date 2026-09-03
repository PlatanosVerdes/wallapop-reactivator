FROM golang:1.26-alpine AS build

WORKDIR /src
ARG VERSION=local
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Static binary: the runtime image has no libc to link against.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.buildVersion=${VERSION}" -o /out/wallapop ./cmd/wallapop

FROM alpine:3.21

WORKDIR /app
RUN apk add --no-cache tzdata ca-certificates
COPY --from=build /out/wallapop /usr/local/bin/wallapop

# The session and the last run live here — mount it or the session is lost on redeploy.
VOLUME /data

ENV WALLA_LOG_JSON=1 \
    WALLA_DATA_DIR=/data \
    WALLA_PORT=8000

EXPOSE 8000

HEALTHCHECK --interval=60s --timeout=10s --start-period=30s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8000/healthz >/dev/null || exit 1

ENTRYPOINT ["wallapop"]
CMD ["serve"]
