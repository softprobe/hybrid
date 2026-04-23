# syntax=docker/dockerfile:1
# Build context: e2e/upstream/
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/upstream .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/upstream /usr/local/bin/upstream
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/upstream"]
