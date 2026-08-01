# Nexus API backend — image for Render (or any Docker host).
#
# Notes:
#  - All data files (companies.json, cities index, …) are go:embedded, so
#    the binary is self-contained.
#  - Playwright browsers for real auto-apply are NOT installed; search,
#    queue, dry-run, and the full API work without them. To enable real
#    applies later, run `cmd/pwinstall` in a browser-capable base image.
#  - Runs `nexus --api` and honors the PORT env var Render injects.
#  - Runs as root so a Render persistent disk mounted at NEXUS_HOME is
#    always writable.

FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata

# Cache module downloads before copying sources.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# modernc.org/sqlite is pure Go → CGO_ENABLED=0 yields a static binary.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nexus .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /var/lib/nexus
COPY --from=build /out/nexus /usr/local/bin/nexus
ENV NEXUS_HOME=/var/lib/nexus
EXPOSE 8080
CMD ["nexus", "--api"]

CMD ["--api", "--api-port", "8080"]