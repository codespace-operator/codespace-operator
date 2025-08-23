# controller Dockerfile
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/session-controller/session-controller.go cmd/session-controller/session-controller.go
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /workspace/session-controller ./cmd/session-controller

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/session-controller /session-controller
USER 65532:65532

# Environment variables with defaults
ENV CODESPACE_CONTROLLER_METRICS_ADDR="0"
ENV CODESPACE_CONTROLLER_PROBE_ADDR=":8081"
ENV CODESPACE_CONTROLLER_ENABLE_LEADER_ELECTION="false"
ENV CODESPACE_CONTROLLER_LEADER_ELECTION_ID="a51c5837.codespace.dev"
ENV CODESPACE_CONTROLLER_SECURE_METRICS="true"
ENV CODESPACE_CONTROLLER_ENABLE_HTTP2="false"
ENV CODESPACE_CONTROLLER_SESSION_NAME_PREFIX="cs-"
ENV CODESPACE_CONTROLLER_FIELD_OWNER="codespace-operator"
ENV CODESPACE_CONTROLLER_DEBUG="false"

# exec form only; let the program's exit code drive health
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/session-controller", "--health-check"]

ENTRYPOINT ["/session-controller"]
