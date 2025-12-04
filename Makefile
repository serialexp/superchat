.PHONY: test coverage coverage-html coverage-lcov coverage-protocol coverage-summary fuzz clean build run-server run-client run-website website docker-build docker-build-push docker-build-server docker-build-website docker-run docker-push docker-stop

# Run all tests
test:
	go test ./... -race

# Generate coverage for all packages (combined)
coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out

# Generate HTML coverage report (combined)
coverage-html:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Generate separate lcov files per package
coverage-lcov:
	@echo "Generating coverage for protocol package..."
	go test ./pkg/protocol/... -coverprofile=protocol.out -covermode=atomic
	gcov2lcov -infile=protocol.out -outfile=protocol.lcov

	@echo "Generating coverage for server package..."
	go test ./pkg/server/... -coverprofile=server.out -covermode=atomic
	gcov2lcov -infile=server.out -outfile=server.lcov

	@echo "Generating coverage for client package..."
	go test ./pkg/client/... -coverprofile=client.out -covermode=atomic
	gcov2lcov -infile=client.out -outfile=client.lcov

	@echo "Generating coverage for database package..."
	go test ./pkg/database/... -coverprofile=database.out -covermode=atomic
	gcov2lcov -infile=database.out -outfile=database.lcov

	@echo ""
	@echo "LCOV coverage reports generated:"
	@echo "  - protocol.lcov  (pkg/protocol)"
	@echo "  - server.lcov    (pkg/server)"
	@echo "  - client.lcov    (pkg/client)"
	@echo "  - database.lcov  (pkg/database)"

# Check protocol coverage (must be at least 85%)
coverage-protocol:
	go test ./pkg/protocol/... -coverprofile=protocol.out -covermode=atomic
	@COVERAGE=$$(go tool cover -func=protocol.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Protocol coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE < 85" | bc -l) -eq 1 ]; then \
		echo "ERROR: Protocol coverage must be at least 85%"; \
		exit 1; \
	fi

# Show coverage summary for each package
coverage-summary:
	@echo "=== Protocol Coverage ==="
	@go tool cover -func=protocol.out | grep total || echo "Run 'make coverage-lcov' first"
	@echo ""
	@echo "=== Server Coverage ==="
	@go tool cover -func=server.out | grep total || echo "Run 'make coverage-lcov' first"
	@echo ""
	@echo "=== Client Coverage ==="
	@go tool cover -func=client.out | grep total || echo "Run 'make coverage-lcov' first"
	@echo ""
	@echo "=== Database Coverage ==="
	@go tool cover -func=database.out | grep total || echo "Run 'make coverage-lcov' first"

# Run fuzzing
fuzz:
	go test ./pkg/protocol -fuzz=FuzzDecodeFrame -fuzztime=5m

# Build server, terminal client, and GUI client
build:
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building with version: $$VERSION"; \
	go build -ldflags="-X main.Version=$$VERSION" -o superchat-server ./cmd/server; \
	go build -ldflags="-X main.Version=$$VERSION" -o superchat ./cmd/client; \
	go build -ldflags="-X main.Version=$$VERSION" -o superchat-gui ./cmd/client-gui; \
	echo "✓ Built: superchat-server, superchat, superchat-gui"

# Run server
run-server:
	go run ./cmd/server

# Run client
run-client:
	go run ./cmd/client

# Build web client and copy to website
website:
	@echo "Building web client..."
	cd web-client && npm run build
	@echo "Copying to website/public/app/..."
	cp web-client/index.html website/public/app/
	cp web-client/dist/main.js website/public/app/dist/
	cp web-client/dist/main.js.map website/public/app/dist/
	@echo "Web client built and copied successfully!"

# Run website dev server
run-website:
	cd website && npm run dev

# Docker commands
docker-build:
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building Docker images with version: $$VERSION"; \
	VERSION=$$VERSION depot bake --load

docker-build-push:
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building and pushing Docker images with version: $$VERSION"; \
	VERSION=$$VERSION depot bake --push

# Build only server image
docker-build-server:
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building server Docker image with version: $$VERSION"; \
	VERSION=$$VERSION depot bake --load server

# Build only website image
docker-build-website:
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	echo "Building website Docker image with version: $$VERSION"; \
	VERSION=$$VERSION depot bake --load website

docker-run:
	docker run -d \
		--name superchat \
		-p 6465:6465 \
		-v superchat-data:/data \
		aeolun/superchat:latest

docker-push:
	@echo "Use 'make docker-build-push' instead - depot requires --push during build"
	@exit 1

docker-stop:
	docker stop superchat || true
	docker rm superchat || true

# Clean coverage files and binaries
clean:
	rm -f coverage.out coverage.html
	rm -f protocol.out protocol.lcov
	rm -f server.out server.lcov
	rm -f client.out client.lcov
	rm -f database.out database.lcov
	rm -f superchat-server superchat superchat-gui
