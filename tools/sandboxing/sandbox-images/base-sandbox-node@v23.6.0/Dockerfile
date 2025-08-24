# Build stage - compile Go server
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Copy Go module files
COPY go.mod go.sum* ./

# Download dependencies (if any)
RUN go mod download

# Copy source code
COPY main.go ./

# Build the Go HTTP server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o sandbox-server main.go

# Runtime stage - Node.js with native TypeScript support
FROM node:23.6.0-alpine AS runtime

# Install wget for health checks
RUN apk add --no-cache wget

# Create a non-root user for security
RUN addgroup -g 1001 -S sandbox && \
    adduser -u 1001 -S sandbox -G sandbox

# Set working directory
WORKDIR /app

# Copy the compiled Go binary from builder stage
COPY --from=builder /build/sandbox-server ./sandbox-server

# Make sure the binary is executable
RUN chmod +x ./sandbox-server

# Switch to non-root user
USER sandbox

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 -O /dev/null http://localhost:8080/health || exit 1

# Run the HTTP server
CMD ["./sandbox-server"]