APP=vmcli

build:
	go build -o bin/$(APP) .

test:
	go test ./...

lint:
	gofmt -w .
	go vet ./...

release:
	GOOS=darwin GOARCH=arm64 go build -o bin/$(APP)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/$(APP)-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build -o bin/$(APP)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -o bin/$(APP)-linux-arm64 .
