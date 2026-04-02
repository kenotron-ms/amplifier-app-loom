BINARY   = loom
DIST     = dist
MODULE   = github.com/ms/amplifier-app-loom
VERSION  = 0.7.1
LDFLAGS  = -ldflags "-X $(MODULE)/internal/api.Version=$(VERSION) -s -w"

.PHONY: build run install-svc uninstall-svc test clean cross ui release

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
