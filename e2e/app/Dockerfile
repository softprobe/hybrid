# syntax=docker/dockerfile:1
# Build context: e2e/app/
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/app .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/app /usr/local/bin/app
ENV EGRESS_PROXY_URL=http://softprobe-proxy:8084
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/app"]
