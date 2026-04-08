# syntax=docker/dockerfile:1
# BuildKit cache mounts keep module + compile cache across rebuilds (enable: DOCKER_BUILDKIT=1).

# ---- Build stage ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 go build -o /app/devopscore ./cmd/api

# ---- Runtime stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/devopscore .
COPY templates/ ./templates/

EXPOSE 8080

CMD ["./devopscore"]
