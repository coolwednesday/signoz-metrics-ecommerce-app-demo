# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies
ENV GOTOOLCHAIN=auto
RUN go mod download

# Copy source code
COPY . .

# Ensure go.sum is up to date and build
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ecommerce-app .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/ecommerce-app .
COPY --from=builder /app/schema.sql .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./ecommerce-app"]

