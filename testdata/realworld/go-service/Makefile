BINARY  := bin/api
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build run test lint docker up down migrate

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/api

run:
	set -a; . ./.env; set +a; go run ./cmd/api

test:
	go test -race ./...

lint:
	golangci-lint run

docker:
	docker build -t ledger:$(VERSION) --build-arg VERSION=$(VERSION) .

up:
	docker compose up -d

down:
	docker compose down

migrate:
	psql "$$DB_URL" -f migrations/0001_init.sql
