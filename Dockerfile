# Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Install build dependencies if necessary (bookworm has gcc by default)
# COPY go.mod go.sum ./
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=1 is required for go-sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o infokeep .

# Runtime stage
FROM debian:bookworm-slim

WORKDIR /app

# Install necessary runtime libraries for sqlite (usually not needed if dynamically linked correctly on same distro, but good practice to have ca-certificates)
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the binary from the builder stage
COPY --from=builder /app/infokeep .

# Copy web assets (templates and static files)
COPY --from=builder /app/web ./web

# Create the uploads directory structure
RUN mkdir -p web/static/uploads

# Expose the application port
EXPOSE 8080

# Run the application
CMD ["./infokeep"]
