# Simple Makefile for go

build:
	go build

clean:
	go clean

run:
	go run .

install:
	go install .

get:
	go get

update:
	go get -u

test:
	go test -race ./...

vet:
	go vet ./...

release:
	./release.sh
