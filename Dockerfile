# Use the official Golang image to create a build artifact.
# This is known as a multi-stage build.
FROM golang:1.24-alpine as builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# CGO_ENABLED=0 is important for a static build
# -ldflags="-w -s" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o bbscope main.go

# Start a new stage from scratch for a smaller image
FROM alpine:latest

# Set the Current Working Directory inside the container
WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/bbscope .

# Expose port 8080 to the outside world (if your app is a web server, which it doesn't seem to be but is good practice)
# EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["./bbscope"]
