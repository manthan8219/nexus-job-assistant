# Stage 1: Build the Go binary
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the static binary (CGO_ENABLED=0 for Alpine compatibility)
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/nexus .

# Stage 2: Runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Create nexus user
RUN adduser -D -h /home/nexus nexus

# Create the data directory
RUN mkdir -p /home/nexus/.nexus && chown -R nexus:nexus /home/nexus/.nexus

COPY --from=builder /app/nexus /usr/local/bin/nexus
COPY --from=builder /app/data /app/data

# Default data directory volume
VOLUME /home/nexus/.nexus

EXPOSE 8080

USER nexus
WORKDIR /home/nexus

ENTRYPOINT ["nexus"]
CMD ["--api", "--api-port", "8080"]