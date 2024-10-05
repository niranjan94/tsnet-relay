# Start from the official Go image
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -v -a -o tsnet-relay .

# Start a new stage from scratch
FROM alpine:latest

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates

# Create a non-root user
RUN addgroup -g 1000 app && \
    adduser -u 1000 -G app -s /bin/sh -D app

# Set the working directory
WORKDIR /home/app

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/tsnet-relay .

# Copy the config file
COPY config.json .

# Chown all the files to the app user
RUN chown -R app:app /home/app

# Switch to non-root user
USER app

# Command to run the executable
ENTRYPOINT ["./tsnet-relay"]
