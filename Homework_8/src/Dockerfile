FROM --platform=linux/amd64 golang:1.24-alpine AS build
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o openapi .

FROM --platform=linux/amd64 alpine:latest
ENV GIN_MODE=release
COPY --from=build /app/openapi /openapi
EXPOSE 8080
ENTRYPOINT ["/openapi"]