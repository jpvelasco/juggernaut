.PHONY: build test lint clean

build:
	go build -ldflags "-X github.com/jpvelasco/juggernaut/cmd.Version=$(shell cat VERSION)" -o bin/juggernaut .

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
