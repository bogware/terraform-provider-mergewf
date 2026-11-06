.PHONY: build test vet fmt generate api-coverage

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

generate:
	go -C tools generate -tags generate .

api-coverage:
	go run ./cmd/check-api-coverage
