# syntax=docker/dockerfile:1

# --- build the Go server -------------------------------------------------
FROM golang:1.27-alpine AS gobuild
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cvrenderer ./cmd/server

# --- fetch the typst binary ----------------------------------------------
FROM alpine:3.24 AS typst
ARG TYPST_VERSION=0.12.0
RUN apk add --no-cache curl xz \
 && curl -sSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" -o /tmp/typst.tar.xz \
 && tar -xf /tmp/typst.tar.xz -C /tmp \
 && install -m 0755 /tmp/typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst

# --- runtime -------------------------------------------------------------
FROM alpine:3.24
RUN adduser -D -u 10001 app \
 && mkdir -p /app/templates /app/data \
 && chown -R app:app /app

COPY --from=gobuild /out/cvrenderer /usr/local/bin/cvrenderer
COPY --from=typst  /usr/local/bin/typst /usr/local/bin/typst
COPY templates/ /app/templates/
COPY fonts/ /app/fonts/
# Vendored Typst packages (e.g. cmarker) so `@preview/...` imports resolve
# offline, with no network access at render time.
COPY typst-packages/ /app/typst-packages/
# The sample CV (+ its placeholder photo) so the service works out of the box;
# override or add more by mounting a volume at /app/data (see
# docker-compose.yml). Each data/<name>.yaml is served at
# GET /cv/<template>/<name>.pdf.
COPY data/ /app/data/
RUN chown -R app:app /app

ENV PORT=8080 \
    ROOT=/app \
    TEMPLATE_DIR=templates \
    DATA_DIR=data \
    FONT_DIR=fonts \
    PACKAGE_CACHE_DIR=typst-packages \
    TYPST_BIN=typst \
    RENDER_TIMEOUT=30s \
    MAX_CONCURRENT_RENDERS=1 \
    RENDER_QUEUE_TIMEOUT=10s \
    CACHE_TTL=24h \
    CACHE_MAX_ENTRIES=64 \
    ALLOW_INDEXING=false

USER app
WORKDIR /app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz >/dev/null 2>&1 || exit 1
ENTRYPOINT ["cvrenderer"]
