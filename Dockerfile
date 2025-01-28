# Build stage
FROM golang:1.23.4-alpine3.21 AS builder
WORKDIR /app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

RUN apk add curl
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.1/migrate.linux-amd64.tar.gz | tar xvz

COPY . .
RUN go build -v -o main main.go

# Run stage
FROM alpine:3.21
WORKDIR /app
COPY --from=builder app/main .
COPY --from=builder app/migrate .
RUN apk add --no-cache tzdata
COPY app.env .
COPY internal/db/migrations ./migrations/
COPY start.sh .
RUN chmod +x start.sh

EXPOSE 8080
CMD ["/app/main"]
ENTRYPOINT ["/app/start.sh"]