STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2025.1.1

.PHONY: build check check-go lint test run

build:                     ## build the binary
	CGO_ENABLED=0 go build -trimpath -o bin/recall ./cmd/recall

check: check-go            ## everything CI runs

check-go:                  ## gofmt, vet, staticcheck, our analyzers, tests
	test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go vet ./...
	go run $(STATICCHECK) ./...
	go run ./cmd/lint ./...
	go test ./...

lint:                      ## only our lint rules
	go run ./cmd/lint ./...

test:
	go test ./...

run: build                 ## serve locally on 8471
	./bin/recall serve --listen 127.0.0.1:8471
