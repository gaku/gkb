BINARY = gkb
INSTALL_DIR = $(HOME)/bin

build:
	go build -o $(BINARY) .

install: build
	install -d $(INSTALL_DIR)
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)

clean:
	rm -f $(BINARY)

.PHONY: build install clean
