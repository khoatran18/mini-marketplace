# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /app/cmd

#RUN go build -v -o /usr/local/bin/web .
#
#CMD ["web"]

#RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -o /app/cmd/web .
RUN go build -v -o /app/cmd/web .


# Stage 2: Runtime
FROM alpine:3.18

COPY --from=builder /app/cmd/web /usr/local/bin/web
COPY --from=builder /app/cmd/.env /usr/local/bin/.env

WORKDIR /usr/local/bin

CMD ["web"]

