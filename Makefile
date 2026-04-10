APP=site-sentry

.PHONY: run test build fmt tidy

run:
	go run ./cmd/site-sentry

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/site-sentry

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
