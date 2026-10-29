.PHONY: prototype-columns help build install run test update-golden vet fmt gate e2e e2e-live \
	fixtures-up fixtures-record fixtures-down demo

help:
	@echo "build            build ./navigo"
	@echo "install          go install navigo into GOBIN (default: GOPATH/bin)"
	@echo "run              go run ./cmd/navigo (ARGS=... passes arguments)"
	@echo "test             go test ./..."
	@echo "update-golden    rewrite the golden frames in internal/ui"
	@echo "vet              go vet ./..."
	@echo "fmt              golangci-lint fmt"
	@echo "gate             the full CI gate in Docker (tools/ci/local.sh)"
	@echo "e2e              layer B against a throwaway Navidrome (Docker)"
	@echo "e2e-live         layer B against your own server, from .env.e2e"
	@echo "fixtures-up      generate the fixture library and start Navidrome"
	@echo "fixtures-record  seed and record the fixtures"
	@echo "fixtures-down    stop Navidrome and drop its data"
	@echo "demo             record docs/media/demo.gif (Docker)"

build:
	go build -o navigo ./cmd/navigo

install:
	go install ./cmd/navigo

run:
	go run ./cmd/navigo $(ARGS)

test:
	go test ./...

update-golden:
	go test ./internal/ui -update

vet:
	go vet ./...
	go vet -tags e2e ./e2e

fmt:
	golangci-lint fmt

gate:
	tools/ci/local.sh

e2e:
	tools/e2e/run.sh

e2e-live:
	go test -tags e2e ./e2e

fixtures-up:
	tools/fixtures.sh up

fixtures-record:
	tools/fixtures.sh record

fixtures-down:
	tools/fixtures.sh down

demo:
	tools/demo/demo.sh

prototype-columns:
	tools/prototype-columns.sh
