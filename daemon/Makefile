.PHONY: lint systemd clean-systemd update-systemd stop-systemd uinput

lint:
	cd cli && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..
	cd libquava && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..
	cd quavad && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..

# Install a per-user, socket-activated quavad. The socket starts quavad the
# first time the CLI connects, and the daemon then keeps running normally.
systemd: uinput
	mkdir -p "$(HOME)/.local/bin" "$(HOME)/.config/quava" "$(HOME)/.config/systemd/user"
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	install -m 0644 systemd/quavad.service "$(HOME)/.config/systemd/user/quavad.service"
	install -m 0644 systemd/quavad.socket "$(HOME)/.config/systemd/user/quavad.socket"
# 	systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XDG_SESSION_TYPE XAUTHORITY DBUS_SESSION_BUS_ADDRESS
	systemctl --user daemon-reload
	systemctl --user enable --now quavad.socket
	systemctl --user restart quavad.service
	systemctl --user restart quavad.socket
	@if ! echo ":$$PATH:" | grep -q ":$(HOME)/.local/bin:"; then \
		echo 'export PATH="$(HOME)/.local/bin:$$PATH"' >> "$(HOME)/.bashrc"; \
		echo "Added $(HOME)/.local/bin to PATH in .bashrc"; \
	fi

	@echo "Quava installed to systemd."


uinput:
	sudo groupadd -f quava-input
	sudo usermod -aG quava-input "$${USER}"
	echo 'KERNEL=="uinput", GROUP="quava-input", MODE="0660"' | sudo tee /etc/udev/rules.d/99-quava-uinput.rules
	sudo udevadm control --reload-rules
	sudo udevadm trigger --name-match=uinput
	@echo "uinput permissions configured. Log out and back in for group membership to take effect."

# Update the systemd binaries and restart the services to update quava, quavad, and the socket. 
update-systemd:
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	systemctl --user daemon-reload
	-systemctl --user restart quavad.service
	systemctl --user restart quavad.socket

	@echo "Quava updated in systemd."

# Full purge of Quava from systemd, including the binary, config, and systemd unit files.
clean-systemd:
	systemctl --user disable --now quavad.socket
	rm -f "$(HOME)/.config/systemd/user/quavad.service" "$(HOME)/.config/systemd/user/quavad.socket"
	rm -f "$(HOME)/.local/bin/quavad"
	rm -f "$(HOME)/.local/bin/quava"
	rm -rf "$(HOME)/.config/quava"
	systemctl --user daemon-reload

	@echo "Quava uninstalled from systemd."

# Stop the systemd service, I use KDE-Connect (until Quava is mature enough)
stop-systemd:
	systemctl --user stop quavad.service
	systemctl --user daemon-reload

	@echo "Quava service stopped."

