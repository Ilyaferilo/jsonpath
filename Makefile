vet:
	@go vet ./...
.PHONY: vet

coverage:
	go test -coverprofile=coverage.out ./... -race -covermode=atomic
	go tool cover -html coverage.out
.PHONY: coverage

test:
	go clean -testcache
	go test ./... -race -count=100
.PHONY: test
