# Relayent build targets.
BINDIR := bin
LDFLAGS := -s -w

.PHONY: all relay bridge test vet clean cross

all: relay bridge

relay:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-relay ./relay

bridge:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge ./bridge

vet:
	go vet ./...

test:
	go test ./...

# Cross-compile the bridge for the common desktop targets users download.
cross:
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge-darwin-arm64  ./bridge
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge-darwin-amd64  ./bridge
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge-linux-amd64   ./bridge
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge-linux-arm64   ./bridge
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/relayent-bridge-windows-amd64.exe ./bridge

clean:
	rm -rf $(BINDIR)
