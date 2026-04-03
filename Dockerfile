# syntax=docker/dockerfile:1.7

FROM golang:1.24-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/archon ./cmd/archon

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    docker.io \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/archon /usr/local/bin/archon

EXPOSE 8080
ENTRYPOINT ["archon"]
CMD ["start"]
