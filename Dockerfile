# --- Build Stage ---
FROM golang:1.24-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the relay server binary with optimizations for a small size
# CGO_ENABLED=0 is important for a static binary
# -ldflags="-s -w" strips debug symbols
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-s -w" -o /relay-server ./cmd/relay-server

# --- Final Stage ---
FROM scratch

# Copy the compiled binary from the builder stage
COPY --from=builder /relay-server /relay-server

# Expose the default port
EXPOSE 8080

# Set the entrypoint for the container
ENTRYPOINT ["/relay-server"]
