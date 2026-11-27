.PHONY: run seed build test vet tidy

run:
	go run ./cmd/api

seed:
	go run ./cmd/seed

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -o bin/seed ./cmd/seed

test:
	go test ./... -race -count=1

vet:
	go vet ./...

tidy:
	go mod tidy
