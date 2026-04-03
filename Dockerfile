# syntax=docker/dockerfile:1

# ── Stage 1: Build Svelte frontend ─────────────────────────────────────────────
FROM oven/bun:1-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# ── Stage 2: Build Go binary (cross-compiles for target platform) ─────────────
FROM golang:1.26.0-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the built frontend so Go's embed directive finds it
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} CGO_ENABLED=0 go build -o foodtracker .

# ── Stage 3: Minimal runtime ──────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/foodtracker /foodtracker
EXPOSE 8080
ENTRYPOINT ["/foodtracker"]
