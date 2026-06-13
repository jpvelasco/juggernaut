.PHONY: build test lint clean codacy codacy-sync

build:
	go build -ldflags "-X github.com/jpvelasco/juggernaut/v4/cmd.Version=$(shell cat VERSION)" -o bin/juggernaut .

test:
	go test ./... -v

lint:
	golangci-lint run ./...

# Local Codacy parity check (WSL). Sync rules from server first: make codacy-sync
codacy:
	wsl -e bash -lic "cd /mnt/f/source/juggernaut && chmod +x scripts/codacy-full.sh scripts/codacy-sync.sh scripts/codacy/patch-eslint.sh && ./scripts/codacy-full.sh"

codacy-sync:
	wsl -e bash -lic "cd /mnt/f/source/juggernaut && chmod +x scripts/codacy-sync.sh scripts/codacy/patch-eslint.sh && ./scripts/codacy-sync.sh"

clean:
	rm -rf bin/
