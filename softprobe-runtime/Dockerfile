# syntax=docker/dockerfile:1
# Unified runtime HTTP server + `softprobe` CLI in one image.
# Default: start the server. `docker run … --version` runs the CLI (PD5.3a smoke).

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.0.0-dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
	-ldflags "-s -w -X softprobe-runtime/internal/version.Version=${VERSION}" \
	-o /out/softprobe-runtime .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
	-ldflags "-s -w -X softprobe-runtime/internal/version.Version=${VERSION}" \
	-o /out/softprobe ./cmd/softprobe

FROM alpine:3.20
# wget: same probe style as e2e compose (`wget -qO- http://127.0.0.1:8080/health`).
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/softprobe-runtime /usr/local/bin/softprobe-runtime
COPY --from=build /out/softprobe /usr/local/bin/softprobe
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh /usr/local/bin/softprobe-runtime /usr/local/bin/softprobe
ENV SOFTPROBE_LISTEN_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
