.PHONY: lint systemd clean-systemd update-systemd stop-systemd uinput rm-uinput reply-prompt

CC := gcc
PKG_CONFIG := pkg-config

lint:
	cd cli && go mod tidy && go vet ./... && go fmt ./... && golangci-lint run && staticcheck ./... && cd ..
	cd libquava && go mod tidy && go vet ./... && go fmt ./... && golangci-lint run && staticcheck ./... && cd ..
	cd quavad && go mod tidy && go vet ./... && go fmt ./... && golangci-lint run && staticcheck ./... && cd ..
	cd quavad && out="$$(deadcode ./... libquava/...)" && if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

# Install a per-user, socket-activated quavad. The socket starts quavad the
# first time the CLI connects, and the daemon then keeps running normally.
systemd: uinput reply-prompt
	mkdir -p "$(HOME)/.local/bin" "$(HOME)/.config/quava" "$(HOME)/.config/systemd/user"
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	install -m 0644 systemd/quavad.service "$(HOME)/.config/systemd/user/quavad.service"
	install -m 0644 systemd/quavad.socket "$(HOME)/.config/systemd/user/quavad.socket"

	mkdir -p "$(HOME)/.local/share/bash-completion/completions" "$(HOME)/.local/share/zsh/site-functions"
	"$(HOME)/.local/bin/quava" completion bash > "$(HOME)/.local/share/bash-completion/completions/quava"
	"$(HOME)/.local/bin/quava" completion zsh > "$(HOME)/.local/share/zsh/site-functions/_quava"

# 	systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XAUTHORITY XDG_RUNTIME_DIR XDG_SESSION_TYPE

	systemctl --user daemon-reload
	systemctl --user enable --now quavad.socket
	systemctl --user restart quavad.service
	systemctl --user restart quavad.socket
	@if ! echo ":$$PATH:" | grep -q ":$(HOME)/.local/bin:"; then \
		echo 'export PATH="$(HOME)/.local/bin:$$PATH"' >> "$(HOME)/.bashrc"; \
		echo "Added $(HOME)/.local/bin to PATH in .bashrc"; \
	fi

	@echo "Quava installed to systemd."

# Notification quick-replies need a small native GTK4 dialog instead of
# shelling out to zenity. Best-effort: if gtk4 dev files are missing, skip it
# rather than fail the whole install (quick-reply just won't be available).
reply-prompt:
	@mkdir -p "$(HOME)/.local/bin"
	@if $(PKG_CONFIG) --exists gtk4 2>/dev/null; then \
		$(CC) $$($(PKG_CONFIG) --cflags gtk4) -O2 -Wall -Wextra -o "$(HOME)/.local/bin/quava-reply-prompt" helpers/reply_prompt.c $$($(PKG_CONFIG) --libs gtk4) && \
		echo "Built quava-reply-prompt (notification quick-reply dialog)."; \
	else \
		echo "gtk4 development files not found; skipping quava-reply-prompt (notification quick-reply will be unavailable)."; \
	fi


# 	uinput permissions configuration
uinput:
	sudo groupadd -f quava-input
	sudo usermod -aG quava-input "$${USER}"
	echo 'KERNEL=="uinput", GROUP="quava-input", MODE="0660"' | sudo tee /etc/udev/rules.d/99-quava-uinput.rules
	sudo udevadm control --reload-rules
	sudo udevadm trigger --name-match=uinput
	@echo "uinput permissions configured. Log out and back in for group membership to take effect."

# 	Remove uinput permissions configuration
rm-uinput:
	sudo gpasswd -d "$(USER)" quava-input
	sudo rm -f /etc/udev/rules.d/99-quava-uinput.rules
	sudo udevadm control --reload-rules
	sudo udevadm trigger --name-match=uinput
	sudo groupdel quava-input


# Update the systemd binaries and restart the services to update quava, quavad, and the socket. 
update-systemd: reply-prompt
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	systemctl --user daemon-reload
	systemctl --user restart quavad.service
	systemctl --user restart quavad.socket

	@echo "Quava updated in systemd."

# Full purge of Quava from systemd, including the binary, config, and systemd unit files.
clean-systemd: rm-uinput
	# Stop and remove systemd services
	systemctl --user disable --now quavad.socket
	rm -f "$(HOME)/.config/systemd/user/quavad.service" "$(HOME)/.config/systemd/user/quavad.socket"
	rm -f "$(HOME)/.local/bin/quavad"
	rm -f "$(HOME)/.local/bin/quava"
	rm -f "$(HOME)/.local/bin/quava-reply-prompt"
	rm -f "$(HOME)/.local/share/bash-completion/completions/quava"
	rm -f "$(HOME)/.local/share/zsh/site-functions/_quava"
	rm -rf "$(HOME)/.config/quava"
	systemctl --user daemon-reload


	@echo "Quava uninstalled from systemd."
# Stop the systemd service, I use KDE-Connect (until Quava is mature enough)
stop-systemd:
	systemctl --user stop quavad.service
	systemctl --user daemon-reload

	@echo "Quava service stopped."

