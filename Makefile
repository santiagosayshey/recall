STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2025.1.1

.PHONY: build check check-go check-integration lint test run

build:                     ## build the binary
	CGO_ENABLED=0 go build -trimpath -o bin/recall ./cmd/recall

check: check-go check-integration  ## everything CI runs

check-go:                  ## gofmt, vet, staticcheck, our analyzers, tests
	test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go vet ./...
	go run $(STATICCHECK) ./...
	go run ./cmd/lint ./...
	go test ./...

check-integration:         ## build the image and replay the captured webhooks through it
	docker build --build-arg VERSION=local -t recall:local .
	RECALL_IMAGE=recall:local go test -count=1 -tags integration ./test/

lint:                      ## only our lint rules
	go run ./cmd/lint ./...

test:
	go test ./...

run: build                 ## serve locally on 8471
	./bin/recall serve --listen 127.0.0.1:8471
