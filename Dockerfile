# Build stage
FROM golang:1.24.0-alpine3.21 AS builder
WORKDIR /app

# Install ca-certificates for SSL
RUN apk add --no-cache ca-certificates git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Run stage
FROM alpine:3.21
WORKDIR /app

# Install ca-certificates for SSL calls
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/main .

# Copy service account file
COPY service-account-file.json .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./main"]