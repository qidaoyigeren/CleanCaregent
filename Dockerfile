FROM golang:1.26.4-alpine AS build

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -o /out/cleancare ./cmd/server

FROM alpine:3.22
RUN adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=build /out/cleancare /app/cleancare
EXPOSE 8080
ENTRYPOINT ["/app/cleancare"]
