BINARY := code-shield-server
FRONTEND_DIR := frontend
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X 'main.Version=$(VERSION)'

.PHONY: all build install frontend backend clean run test lint lint-arch

all: build

install: frontend/node_modules

frontend/node_modules:
	cd $(FRONTEND_DIR) && npm install

frontend: frontend/dist

frontend/dist:
	cd $(FRONTEND_DIR) && npm run build

backend: $(BINARY)

$(BINARY):
	go build -ldflags "$(LDFLAGS)" -o $(BINARY)

clean:
	rm -rf frontend/dist $(BINARY)

run: build
	./$(BINARY)

test:
	go test ./...

lint:
	make lint-arch
	cd frontend && npm run lint

lint-arch:
	@echo "Checking architecture boundaries..."
	@if grep -rn "models\.DB" services/engines/; then \
		echo "❌ ERROR: engines/ should not access models.DB"; exit 1; fi
	@if grep -rn "code-shield/services/engines" services/invoker/; then \
		echo "❌ ERROR: invoker/ should not depend on engines/"; exit 1; fi
	@if grep -rn "code-shield/services/runner" services/engines/; then \
		echo "❌ ERROR: engines/ should not depend on runner/"; exit 1; fi
	@echo "✅ ArchGuard Passed"