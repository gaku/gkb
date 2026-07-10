BINARY = gkb
INSTALL_DIR = $(HOME)/bin

build:
	go build -o $(BINARY) .

install: build
	install -d $(INSTALL_DIR)
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)

install-skill:
	install -d $(HOME)/.claude/skills/gkb
	install -m 644 skills/gkb/SKILL.md $(HOME)/.claude/skills/gkb/SKILL.md
	install -d $(HOME)/.config/opencode/skills/gkb
	install -m 644 skills/gkb/SKILL.md $(HOME)/.config/opencode/skills/gkb/SKILL.md

start:
	systemctl --user start gkb-serve
stop:
	systemctl --user stop gkb-serve

restart:
	systemctl --user restart gkb-serve

status:
	systemctl --user status gkb-serve

clean:
	rm -f $(BINARY)

.PHONY: build install install-skill start stop restart status clean
