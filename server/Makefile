.PHONY: test test-race vet lint build clean

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

build:
	go build ./...

clean:
	go clean
