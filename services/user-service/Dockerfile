# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/web ./cmd

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/web /usr/local/bin/web
USER app
CMD ["web"]
