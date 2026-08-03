.PHONY: build run test fmt vet clean build-linux-amd64 build-linux-arm64

build:
	go build -o puretty .

run:
	go run .

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f puretty puretty-linux-amd64 puretty-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o puretty-linux-amd64 .

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o puretty-linux-arm64 .
