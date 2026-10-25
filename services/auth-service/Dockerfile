# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
# Build context = REPO ROOT (the service imports the shared module via "replace ../../shared"):
#   docker build -f services/auth-service/Dockerfile .
WORKDIR /src
COPY shared/go.mod shared/go.sum ./shared/
COPY services/auth-service/go.mod services/auth-service/go.sum ./services/auth-service/
WORKDIR /src/services/auth-service
RUN go mod download
WORKDIR /src
COPY shared ./shared
COPY services/auth-service ./services/auth-service
WORKDIR /src/services/auth-service
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/web ./cmd

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/web /usr/local/bin/web
ARG SERVICE_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ENV SERVICE_VERSION=$SERVICE_VERSION GIT_COMMIT=$GIT_COMMIT BUILD_TIME=$BUILD_TIME
USER app
CMD ["web"]
