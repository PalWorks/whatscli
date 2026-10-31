# Simple Makefile for go

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -X main.VERSION=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o whatscli .

clean:
	go clean

run:
	go run .

install:
	go install -ldflags "$(LDFLAGS)" .

get:
	go get

update:
	go get -u

test:
	go test -race ./...

vet:
	go vet ./...

lint: vet

release:
	@echo "Releases are built by .github/workflows/release.yml."
	@echo "Tag and push: git tag vX.Y.Z && git push origin vX.Y.Z"
