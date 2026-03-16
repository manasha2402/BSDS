FROM golang:1.24-alpine AS build
RUN apk add --no-cache git

WORKDIR /src
# Copy dependency files from root
COPY go.mod go.sum ./
RUN go mod download

# Copy ONLY this service's code
COPY order-processor/ ./order-processor/

# Build the binary from the service subdirectory
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o server ./order-processor/

FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /src/server .

ENTRYPOINT ["./server"]