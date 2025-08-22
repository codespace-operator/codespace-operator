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

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /workspace/codespace-controller ./cmd/session-controller

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/codespace-controller /codespace-controller
USER 65532:65532

# exec form only; let the program's exit code drive health
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/codespace-controller", "--health-check"]

ENTRYPOINT ["/codespace-controller"]
