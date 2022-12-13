GOPATH := $(HOME)/go
PATH := $(PATH):$(GOPATH)/bin

build:
	GOARCH=amd64 GOOS=linux go build -o bin/twamp-client cmd/main.go
#	GOARCH=amd64 GOOS=linux GCO_ENABLED=0 go build -a -ldflags="-extldflags=-static"  -o bin/twamp-client cmd/main.go

buildrace:
	GOARCH=amd64 GOOS=linux go build -race -o bin/twamp-client cmd/main.go

test:
	go test

clean:
	go clean
	rm bin/twamp-client
