FROM golang:1.24-alpine AS build
RUN apk add --no-cache git

WORKDIR /src
# Copy dependency files from root
COPY go.mod go.sum ./
RUN go mod download

# Copy ONLY the lambda code
COPY lambda/ ./lambda/

# Build the binary — must be named "bootstrap" for provided.al2 runtime
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o bootstrap ./lambda/main.go

FROM public.ecr.aws/lambda/provided:al2
COPY --from=build /src/bootstrap ./bootstrap

ENTRYPOINT ["./bootstrap"]