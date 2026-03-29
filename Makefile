BINARY   = loom
DIST     = dist
MODULE   = github.com/ms/amplifier-app-loom
VERSION  = 0.5.1
LDFLAGS  = -ldflags "-X $(MODULE)/internal/api.Version=$(VERSION) -s -w"

.PHONY: build run install-svc uninstall-svc test clean cross

$(DIST):
	mkdir -p $(DIST)

build: $(DIST)
	go build $(LDFLAGS) -o $(DIST)/$(BINARY) ./cmd/loom/

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
	rm -rf $(DIST)

# Cross-compile for all platforms (CGO_ENABLED=0, tray excluded)
cross: $(DIST)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-amd64   ./cmd/loom/
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-arm64   ./cmd/loom/
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-amd64  ./cmd/loom/
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-arm64  ./cmd/loom/
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(DIST)/$(BINARY)-windows-amd64.exe ./cmd/loom/
	ls -lh $(DIST)/
