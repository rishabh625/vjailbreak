# Build stage
FROM golang:1.24-alpine AS builder

# Install make and other build dependencies
RUN apk add --no-cache make bash git

# Set working directory
WORKDIR /app

# Copy the source code
COPY . .

# Download dependencies
RUN make dep

# Build the application
RUN make config-swagger
RUN make vpwctl

# Final stage
FROM alpine:3.18

# Install ca-certificates and tzdata for proper SSL and timezone support
RUN apk --no-cache add ca-certificates tzdata

# Set working directory
WORKDIR /root/

# Copy the pre-built binary file from the builder stage
COPY --from=builder /app/bin/vpwctl .

# Copy any additional files you need
# Uncomment and modify as needed
COPY --from=builder /app/openapiv3 /opt/platform9/vpwned/openapiv3

# Set the timezone (optional)
ENV TZ=UTC
ENV LOG_LEVEL=error
# Expose port if your application uses a network port
EXPOSE 3001
EXPOSE 3000
EXPOSE 80
EXPOSE 443

# Command to run the executable
CMD ["./vpwctl","serve"]