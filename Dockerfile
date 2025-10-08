# Build stage
FROM golang:1.23-bookworm AS builder

# Install build dependencies (gcc and sqlite are already in bookworm)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 go build -o news .

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy the binary from builder
COPY --from=builder /app/news .
COPY --from=builder /app/sql ./sql
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
COPY --from=builder /app/seed ./seed

# Expose port
EXPOSE 8080

# Run the application
CMD ["./news"]

