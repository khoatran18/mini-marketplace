FROM golang:1.25

WORKDIR /usr/src/web

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /usr/src/web/cmd

RUN go build -v -o /usr/local/bin/web .

CMD ["web"]

