# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/numparser ./cmd

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/numparser /app/numparser
COPY config.example.yml /app/config.example.yml

VOLUME ["/app/db", "/app/public/releases"]

EXPOSE 38888

ENTRYPOINT ["/app/numparser"]
CMD ["-p", "38888"]
