.PHONY: build test vet install

build:
	go build -o bin/procscope ./cmd/procscope

test:
	go test ./...

vet:
	go vet ./...

install:
	go install ./cmd/procscope

