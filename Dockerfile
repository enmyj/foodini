# ── Stage 1: Build Svelte frontend ────────────────────────────────────────────
FROM node:24-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ── Stage 2: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.26.0-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the built frontend so Go's embed directive finds it
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o foodtracker .

# ── Stage 3: Minimal runtime ──────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/foodtracker /foodtracker
EXPOSE 8080
ENTRYPOINT ["/foodtracker"]
