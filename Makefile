BINARY     := led-clock
PREFIX     ?= /usr/local
BINDIR     ?= $(PREFIX)/bin
UNITDIR    ?= /etc/systemd/system
SERVICE    := $(BINARY).service
PREVIEW    ?= docs/preview.png
SOURCES    := $(wildcard *.go) go.mod go.sum
EMULATOR   := led-emulator
EMU_SOURCES := $(wildcard cmd/led-emulator/*) go.mod go.sum

.PHONY: all build test vet fmt preview run emulator emulate install install-service uninstall clean

all: build

build: $(BINARY)

$(BINARY): $(SOURCES)
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Render the clock digits to $(PREVIEW) (shown in the README).
preview:
	PREVIEW_PNG=$(abspath $(PREVIEW)) go test -run TestPreview ./...

run: $(BINARY)
	./$(BINARY)

# The hardware emulator (see README). `make emulate` runs the clock on it;
# pass more flags with ARGS, e.g. make emulate ARGS="-fill spiral".
emulator: $(EMULATOR)

$(EMULATOR): $(EMU_SOURCES)
	go build -o $(EMULATOR) ./cmd/led-emulator

emulate: $(EMULATOR) $(BINARY)
	./$(BINARY) -emulator $(ARGS)

# Install the binary only. Use DESTDIR for staged installs. Run `make` as
# yourself first so `sudo make install` doesn't need Go.
install: $(BINARY)
	install -D -m 755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)

# Install the binary and the systemd unit (pointing at $(BINDIR)), then
# enable and (re)start the service.
install-service: install
	sed 's|/usr/local/bin/|$(BINDIR)/|' $(SERVICE) > $(SERVICE).tmp
	install -D -m 644 $(SERVICE).tmp $(DESTDIR)$(UNITDIR)/$(SERVICE)
	rm -f $(SERVICE).tmp
	systemctl daemon-reload
	systemctl enable $(SERVICE)
	systemctl restart $(SERVICE)

uninstall:
	-systemctl disable --now $(SERVICE)
	rm -f $(DESTDIR)$(UNITDIR)/$(SERVICE) $(DESTDIR)$(BINDIR)/$(BINARY)
	-systemctl daemon-reload

clean:
	rm -f $(BINARY) $(EMULATOR) $(SERVICE).tmp
