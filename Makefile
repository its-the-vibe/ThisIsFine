.PHONY: build test lint ci

build:
	go build -o ThisIsFine .

test:
	go test -v ./...

lint:
	go vet ./...

ci: build test lint
