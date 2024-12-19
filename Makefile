# ==================================================================================== #
## :
## HELPERS:
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

# ==================================================================================== #
## :
## QUALITY CONTROL:
# ==================================================================================== #

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go mod tidy -v
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit:
	go mod verify
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	go test -race -buildvcs -vet=off ./...

## lint: run linters
.PHONY: lint
lint:
	golangci-lint run

# ==================================================================================== #
## :
## DEVELOPMENT:
# ==================================================================================== #

## test: run all tests for the project
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/integration pkg=$1: run all integration tests for the given package
.PHONY: test/integration
test/integration:
	go test -v -race -buildvcs -tags=integration ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## test/bench pkg=$1: run all benchmarks for the given package
.PHONY: test/bench
test/bench:
	go test -v ./... -bench=. -benchmem -run ^$ #gosetup
