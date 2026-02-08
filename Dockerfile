FROM golang:1.24.6-bullseye AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Copy source code (needed for go mod tidy to resolve local packages)
COPY . .

# Download dependencies, tidy, and verify module
# go mod tidy needs source files to resolve local packages
RUN go mod download && go mod tidy && go mod verify

# Build the application
RUN go build -o app .

FROM debian:12.12-slim

# Install ca-certificates for SSL certificate verification
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/app .

# Run the binary
ENTRYPOINT ["./app"]
