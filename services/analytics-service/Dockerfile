# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
# Build context = REPO ROOT (the service imports the shared module via "replace ../../shared"):
#   docker build -f services/analytics-service/Dockerfile .
WORKDIR /src
COPY shared/go.mod shared/go.sum ./shared/
COPY services/analytics-service/go.mod services/analytics-service/go.sum ./services/analytics-service/
WORKDIR /src/services/analytics-service
RUN go mod download
WORKDIR /src
COPY shared ./shared
COPY services/analytics-service ./services/analytics-service
WORKDIR /src/services/analytics-service
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/web ./cmd

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/web /usr/local/bin/web
# Durable spool (ANALYTICS_SPOOL_DIR): a named volume mounted here inherits this ownership
RUN mkdir -p /var/spool/analytics && chown app:app /var/spool/analytics
ARG SERVICE_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ENV SERVICE_VERSION=$SERVICE_VERSION GIT_COMMIT=$GIT_COMMIT BUILD_TIME=$BUILD_TIME
USER app
CMD ["web"]
