.PHONY: lint systemd clean-systemd

lint:
	cd cli && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..
	cd libquava && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..
	cd quavad && go mod tidy && go vet ./... && golangci-lint run && staticcheck ./... && cd ..

# Install a per-user, socket-activated quavad. The socket starts quavad the
# first time the CLI connects, and the daemon then keeps running normally.
systemd:
	mkdir -p "$(HOME)/.local/bin" "$(HOME)/.config/quava" "$(HOME)/.config/systemd/user"
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	install -m 0644 systemd/quavad.service "$(HOME)/.config/systemd/user/quavad.service"
	install -m 0644 systemd/quavad.socket "$(HOME)/.config/systemd/user/quavad.socket"
	systemctl --user daemon-reload
	systemctl --user enable --now quavad.socket
	echo "export PATH=\"$(HOME)/.local/bin:\$$PATH\"" >> "$(HOME)/.bashrc"

	@echo "Quava installed to systemd."


# Update the systemd binaries and restart the services to update quava, quavad, and the socket. 
update-systemd:
	go build -o "$(HOME)/.local/bin/quavad" ./quavad
	go build -o "$(HOME)/.local/bin/quava" ./cli
	systemctl --user daemon-reload
	systemctl --user restart quavad.socket
	systemctl --user restart quavad.service
	
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