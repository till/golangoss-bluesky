.PHONY: test lint build run-dev

test:
	go test ./...

lint:
	go vet ./...
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

build:
	go build -o bot ./cmd/bot

run-dev:
	go run ./cmd/bot
