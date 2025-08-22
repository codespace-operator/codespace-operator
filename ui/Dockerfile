# ui/Dockerfile
FROM node:20-alpine AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json* ui/pnpm-lock.yaml* ./
RUN npm i --silent || true
COPY ui/ ./
RUN npm run build

FROM golang:1.25 AS go
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# move built UI into an embedded path
RUN mkdir -p cmd/gateway/static && cp -r ui/dist/* cmd/gateway/static/
RUN CGO_ENABLED=0 GOOS=linux go build -a -o gateway ./cmd/gateway

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=go /workspace/gateway /gateway
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/gateway"]
