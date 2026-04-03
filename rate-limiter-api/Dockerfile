# Use lightweight Go image
FROM golang:latest

# Set working directory inside container
WORKDIR /app

# Copy go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy entire project
COPY . .

# Build the Go app
RUN go build -o gateway

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./gateway"]