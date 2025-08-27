# 1) Build the SPA
FROM node:20-alpine AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json* ui/pnpm-lock.yaml* ./
# Use npm ci when lockfile exists; fall back to npm i if not
RUN if [ -f package-lock.json ]; then npm ci --no-audit --no-fund; else npm i --silent; fi
COPY ui/ ./
RUN npm run build

# 2) Build the Go server and embed the SPA
FROM golang:1.25 AS go
WORKDIR /workspace

# Go module cache
COPY go.mod go.sum ./
RUN go mod download

# Bring in only the Go sources first (better cache); copy UI assets from the ui stage
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
# Drop the built SPA into a static folder your server serves from
RUN mkdir -p cmd/server/static
COPY --from=ui /src/ui/dist/ cmd/server/static/

# Build a static binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /workspace/codespace-server ./cmd/server

# 3) Minimal runtime image
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go /workspace/codespace-server /codespace-server

USER 65532:65532
EXPOSE 8080

ENV CODESPACE_SERVER_PORT=8080
ENV CODESPACE_SERVER_HOST=""
ENV CODESPACE_SERVER_DEBUG=false
ENV CODESPACE_SERVER_ALLOW_ORIGIN=""

ENTRYPOINT ["/codespace-server"]
