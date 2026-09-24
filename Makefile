BINARY := bin/go-reality-server

.PHONY: build test test-integration cross package fmt vet gen-keys clean

build:
	go build -trimpath -o $(BINARY) ./cmd/server

test:
	go test ./...

test-integration:
	go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v

cross:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server

package:
	./scripts/package.sh

fmt:
	gofmt -w .

vet:
	go vet ./...

gen-keys:
	go run ./cmd/server keys generate

clean:
	rm -rf bin dist
