FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o openapi .

FROM alpine:latest
ENV GIN_MODE=release
COPY --from=build /app/openapi /openapi
EXPOSE 8080
ENTRYPOINT ["/openapi"]