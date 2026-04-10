FROM golang:1.23-bookworm AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /site-sentry ./cmd/site-sentry

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /site-sentry /usr/local/bin/site-sentry
EXPOSE 8080
ENV APP_PORT=8080
CMD ["site-sentry"]
