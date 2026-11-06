.PHONY: build test testacc vet fmt generate api-coverage

build:
	go build ./...

test:
	go test ./...

# Runs against a live tenant and changes it. See docs/guides/acceptance-testing.md.
testacc:
	TF_ACC=1 go test ./internal/provider -run TestAcc -count=1 -v -timeout 30m

vet:
	go vet ./...

fmt:
	gofmt -w .

generate:
	go -C tools generate -tags generate .

api-coverage:
	go run ./cmd/check-api-coverage
