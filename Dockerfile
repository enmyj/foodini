# ── Stage 1: Build Svelte frontend ────────────────────────────────────────────
FROM oven/bun:1-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock* ./
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --frozen-lockfile
COPY frontend/ ./
RUN bun run build

# ── Stage 2: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.26.0-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
# Bring in the built frontend so Go's embed directive finds it
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o foodtracker .

# ── Stage 3: Minimal runtime ──────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/foodtracker /foodtracker
EXPOSE 8080
ENTRYPOINT ["/foodtracker"]
