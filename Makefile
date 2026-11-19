.PHONY: all build build-ui build-go build-universal build-all test run run-mock clean

all: build

build-ui:
	@echo "Building frontend SPA..."
	cd web && npm run build

build-go:
	@echo "Compiling native Go binary..."
	go build -trimpath -o bin/cups-webui ./cmd/cups-webui

build-universal: build-ui
	@echo "Compiling Universal (Apple Silicon + Intel) binary..."
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/cups-webui-arm64 ./cmd/cups-webui
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cups-webui-amd64 ./cmd/cups-webui
	lipo -create -output bin/cups-webui bin/cups-webui-arm64 bin/cups-webui-amd64
	@rm bin/cups-webui-arm64 bin/cups-webui-amd64
	@echo "Universal binary complete: ./bin/cups-webui"
	@lipo -info bin/cups-webui

build: build-universal
	@echo "Build complete: ./bin/cups-webui"

test:
	@echo "Running backend test suite..."
	go test -v ./...

run: build-go
	./bin/cups-webui

run-mock: build-go
	./bin/cups-webui -mock

clean:
	rm -rf bin/
	rm -rf internal/web/dist/
