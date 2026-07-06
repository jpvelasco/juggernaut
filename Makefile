.PHONY: build test lint clean codacy fmt vet test-race test-cover tidy ci

build:
	go build -ldflags "-X github.com/jpvelasco/juggernaut/v5/cmd.Version=$(shell cat VERSION)" -o bin/juggernaut .

test:
	go test ./... -v

test-race:
	go test -race ./... -v

test-cover:
	go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy && go mod verify

ci: tidy fmt vet lint test

# Codacy cloud — check dashboard issues (requires @codacy/codacy-cloud-cli + CODACY_API_TOKEN)
codacy:
	npx @codacy/codacy-cloud-cli issues gh jpvelasco juggernaut --overview

clean:
	rm -rf bin/
