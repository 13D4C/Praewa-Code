WAILS = go run github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
WAILS_TAGS ?= webkit2_41

.PHONY: dev build check test preview frontend-build install-launcher

dev:
	$(WAILS) dev -tags $(WAILS_TAGS)

build:
	$(WAILS) build -tags $(WAILS_TAGS)

install-launcher:
	python3 scripts/install-linux-launcher.py

check:
	cd frontend && npm run check

frontend-build:
	cd frontend && npm run build

test: frontend-build
	go test -race -tags $(WAILS_TAGS) ./...

preview:
	cd frontend && npm run dev
