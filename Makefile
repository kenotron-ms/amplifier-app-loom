BINARY   = loom
DIST     = dist
MODULE   = github.com/ms/amplifier-app-loom
VERSION  = 0.8.5
LDFLAGS  = -ldflags "-X $(MODULE)/internal/api.Version=$(VERSION) -s -w"

.PHONY: build run install-svc uninstall-svc test clean cross ui release dev dev-go dev-ui app app-sync

ui:
	cd ui && npm install && npm run build
	rm -rf web/dist
	cp -r ui/dist web/dist

build: ui $(DIST)
	go build $(LDFLAGS) -o $(DIST)/$(BINARY) ./cmd/loom/

$(DIST):
	mkdir -p $(DIST)

run: build
	./$(DIST)/$(BINARY) _serve

# ── Development: hot-reload Go (air) + Vite dev server in parallel ────────
# Browse http://localhost:5173  →  Vite proxies /api + /ws → Go at :7700
#
# make app      — full build + assemble dist/Loom.app + ad-hoc sign + launch
# make app-sync — fast path used by air: drop new binary → re-sign → relaunch
# make dev      — bootstrap Loom.app then watch: .go save → rebuild → relaunch

app: ui $(DIST)
	CGO_ENABLED=1 go build -ldflags "-X $(MODULE)/internal/api.Version=99.0.0-dev -s -w" -o $(DIST)/$(BINARY) ./cmd/loom/
	@rm -rf dist/Loom.app
	@mkdir -p dist/Loom.app/Contents/MacOS dist/Loom.app/Contents/Resources
	@cp dist/loom dist/Loom.app/Contents/MacOS/loom
	@sed 's/{{VERSION}}/dev/g' packaging/macos/Info.plist > dist/Loom.app/Contents/Info.plist
	@cp packaging/macos/Loom.icns dist/Loom.app/Contents/Resources/ 2>/dev/null || true
	@codesign --force --deep --sign - dist/Loom.app
	@xattr -cr dist/Loom.app
	@open dist/Loom.app
	@printf "\033[32m→ dist/Loom.app launched\033[0m\n"

app-sync:
	@pkill -f "dist/Loom.app/Contents/MacOS/loom" 2>/dev/null || true
	@sleep 0.3
	@cp dist/loom dist/Loom.app/Contents/MacOS/loom
	@codesign --force --deep --sign - dist/Loom.app
	@xattr -cr dist/Loom.app
	@open dist/Loom.app
	@printf "\033[32m→ Loom.app reloaded\033[0m\n"

dev:
	@mkdir -p $(DIST)
	@if [ ! -f web/dist/index.html ]; then \
		printf "\033[33m→ web/dist is empty — building UI assets first...\033[0m\n"; \
		$(MAKE) ui; \
	fi
	@if [ ! -d dist/Loom.app ]; then \
		printf "\033[33m→ building initial Loom.app...\033[0m\n"; \
		$(MAKE) app; \
	fi
	@command -v air >/dev/null 2>&1 || (printf "\033[33m→ Installing air...\033[0m\n" && go install github.com/air-verse/air@latest)
	@printf "\033[33m→ Stopping any running loom service...\033[0m\n"
	@loom stop 2>/dev/null || true
	@printf "\033[32m→ Dev environment starting:\033[0m\n"
	@printf "  \033[36m•\033[0m Go hot-reload (air)    — .go save → rebuild → Loom.app relaunches\n"
	@printf "  \033[36m•\033[0m Vite dev server         — http://localhost:5173\n\n"
	@$(MAKE) -j2 dev-go dev-ui

dev-go:
	air

dev-ui:
	cd ui && npm run dev

install-svc: build
	./$(DIST)/$(BINARY) install
	./$(DIST)/$(BINARY) start

uninstall-svc:
	./$(DIST)/$(BINARY) stop || true
	./$(DIST)/$(BINARY) uninstall

test:
	go test ./...

clean:
	rm -rf $(DIST) web/dist ui/dist

# Cross-compile for all platforms (CGO_ENABLED=0, tray excluded)
cross: ui $(DIST)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-amd64   ./cmd/loom/
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-arm64   ./cmd/loom/
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-amd64  ./cmd/loom/
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-arm64  ./cmd/loom/
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-windows-amd64.exe ./cmd/loom/
	ls -lh $(DIST)/

# Cut a full release: cross-compile → checksums → git tag → GitHub Release
# Usage: make release VERSION=1.2.3
release: cross
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release VERSION=x.y.z"; exit 1; fi
	@echo "--- Generating checksums ---"
	cd $(DIST) && shasum -a 256 \
		$(BINARY)-linux-amd64 \
		$(BINARY)-linux-arm64 \
		$(BINARY)-darwin-amd64 \
		$(BINARY)-darwin-arm64 \
		$(BINARY)-windows-amd64.exe \
		> checksums.txt
	cat $(DIST)/checksums.txt
	@echo "--- Tagging v$(VERSION) ---"
	git tag v$(VERSION)
	git push origin v$(VERSION)
	@echo "--- Creating GitHub Release ---"
	gh release create v$(VERSION) \
		$(DIST)/$(BINARY)-linux-amd64 \
		$(DIST)/$(BINARY)-linux-arm64 \
		$(DIST)/$(BINARY)-darwin-amd64 \
		$(DIST)/$(BINARY)-darwin-arm64 \
		$(DIST)/$(BINARY)-windows-amd64.exe \
		$(DIST)/checksums.txt \
		--title "v$(VERSION)" \
		--generate-notes
	@echo "--- Done: https://github.com/$(shell git remote get-url origin | sed 's/.*github.com\///;s/\.git//') ---"
