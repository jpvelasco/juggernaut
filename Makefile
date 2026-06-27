.PHONY: build test lint clean codacy codacy-sync fmt vet test-race test-cover tidy ci

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

# Local Codacy parity check (WSL). Sync rules from server first: make codacy-sync
codacy:
	wsl -e bash -lic "cd /mnt/f/source/juggernaut && chmod +x scripts/codacy-full.sh scripts/codacy-sync.sh scripts/codacy/patch-eslint.sh && ./scripts/codacy-full.sh"

codacy-sync:
	wsl -e bash -lic "cd /mnt/f/source/juggernaut && chmod +x scripts/codacy-sync.sh scripts/codacy/patch-eslint.sh && ./scripts/codacy-sync.sh"

clean:
	rm -rf bin/
