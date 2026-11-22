GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

# Load .env if present so `make run` picks up DATABASE_URL etc.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: tools generate build run migrate test lint db-up db-down docker

tools: ## install code generators and linters
	go install github.com/a-h/templ/cmd/templ@v0.3.1118
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
	go install github.com/pressly/goose/v3/cmd/goose@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest

generate: ## regenerate sqlc + templ output (commit the result)
	sqlc generate
	templ generate

build: generate
	go build -o bin/server ./cmd/server

run: build
	./bin/server

migrate: build
	./bin/server migrate

test:
	go test ./...

lint:
	go vet ./...
	staticcheck ./...

db-up:
	docker compose up -d --wait db

db-down:
	docker compose down

docker:
	docker build -f docker/Dockerfile -t sny-tracker .
