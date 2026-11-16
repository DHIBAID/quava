#!/usr/bin/env bash
# Install Quava (daemon, CLI, and Linux GUI) from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/DHIBAID/quava/main/install.sh | bash
#
set -euo pipefail

REPO="DHIBAID/quava"
HOME="${HOME:-$(getent passwd "${USER:-$(id -un)}" | cut -d: -f6)}"

BIN_DIR="$HOME/.local/bin"
APPS_DIR="$HOME/.local/share/applications"
ICONS_DIR="$HOME/.local/share/icons/hicolor/192x192/apps"
SYSTEMD_DIR="$HOME/.config/systemd/user"
BASH_COMP_DIR="$HOME/.local/share/bash-completion/completions"
ZSH_COMP_DIR="$HOME/.local/share/zsh/site-functions"

for cmd in curl tar install; do
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "error: $cmd is required but not installed." >&2
		exit 1
	fi
done

if [ "$(uname -s)" != "Linux" ]; then
	echo "error: this installer only supports Linux." >&2
	exit 1
fi

case "$(uname -m)" in
	x86_64) ARCH=amd64 ;;
	aarch64 | arm64) ARCH=arm64 ;;
	*)
		echo "error: unsupported architecture $(uname -m)." >&2
		exit 1
		;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> Looking up the latest Quava release"
FINAL_URL="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")"
TAG="${FINAL_URL##*/}"
if [ -z "$TAG" ] || [ "$TAG" = "latest" ]; then
	echo "error: could not determine the latest release." >&2
	exit 1
fi
VERSION="${TAG#v}"
BASE="https://github.com/$REPO/releases/download/$TAG"
echo "    $TAG"

echo "==> Installing daemon + CLI ($ARCH)"
DAEMON_TARBALL="quava-daemon_${VERSION}_linux_${ARCH}.tar.gz"
curl -fsSL -o "$TMP/daemon.tar.gz" "$BASE/$DAEMON_TARBALL"
tar xzf "$TMP/daemon.tar.gz" -C "$TMP"
PKG_DIR="$TMP/quava-daemon_${VERSION}_linux_${ARCH}"

mkdir -p "$BIN_DIR" "$HOME/.config/quava" "$SYSTEMD_DIR"
install -m 0755 "$PKG_DIR/quavad" "$BIN_DIR/quavad"
install -m 0755 "$PKG_DIR/quava" "$BIN_DIR/quava"
install -m 0644 "$PKG_DIR/systemd/quavad.service" "$SYSTEMD_DIR/quavad.service"
install -m 0644 "$PKG_DIR/systemd/quavad.socket" "$SYSTEMD_DIR/quavad.socket"

echo "==> Installing shell completions"
mkdir -p "$BASH_COMP_DIR" "$ZSH_COMP_DIR"
"$BIN_DIR/quava" completion bash >"$BASH_COMP_DIR/quava"
"$BIN_DIR/quava" completion zsh >"$ZSH_COMP_DIR/_quava"

echo "==> Configuring uinput permissions (requires sudo)"
if ! command -v sudo >/dev/null 2>&1; then
	echo "warning: sudo not found -- skipping uinput setup." >&2
elif ! sudo -v 2>/dev/null; then
	echo "warning: no sudo access -- skipping uinput setup." >&2
	echo "         Run later: sudo groupadd -f quava-input && sudo usermod -aG quava-input \$USER" >&2
else
	sudo groupadd -f quava-input
	sudo usermod -aG quava-input "$USER"
	echo 'KERNEL=="uinput", GROUP="quava-input", MODE="0660"' | sudo tee /etc/udev/rules.d/99-quava-uinput.rules >/dev/null
	sudo udevadm control --reload-rules
	sudo udevadm trigger --name-match=uinput
fi

echo "==> Starting the daemon via systemd"
systemctl --user daemon-reload
systemctl --user enable --now quavad.socket
systemctl --user restart quavad.service quavad.socket

echo "==> Installing the Linux GUI"
if [ "$ARCH" = "amd64" ]; then
	APPIMAGE="quava-desktop_${VERSION}-x86_64.AppImage"
	if curl -fsSL -o "$TMP/quava-gui.AppImage" "$BASE/$APPIMAGE"; then
		chmod +x "$TMP/quava-gui.AppImage"
		install -Dm755 "$TMP/quava-gui.AppImage" "$BIN_DIR/quava-gui"
		(cd "$TMP" && ./quava-gui.AppImage --appimage-extract >/dev/null 2>&1) || true
		if [ -d "$TMP/squashfs-root" ]; then
			install -Dm644 "$TMP/squashfs-root/usr/share/applications/io.quava.Desktop.desktop" \
				"$APPS_DIR/io.quava.Desktop.desktop"
			install -Dm644 "$TMP/squashfs-root/usr/share/icons/hicolor/192x192/apps/io.quava.Desktop.png" \
				"$ICONS_DIR/io.quava.Desktop.png"
		else
			echo "warning: could not unpack the GUI AppImage -- no app-menu entry installed, but $BIN_DIR/quava-gui still works." >&2
		fi
	else
		echo "warning: no GUI AppImage found for $TAG -- skipping." >&2
	fi
else
	echo "note: no prebuilt GUI for $ARCH yet -- skipping." >&2
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -q -t "$HOME/.local/share/icons/hicolor" || true
fi
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database -q "$APPS_DIR" || true
fi

case ":$PATH:" in
	*":$BIN_DIR:"*) ;;
	*) echo "note: add $BIN_DIR to your PATH (e.g. in ~/.bashrc or ~/.zshrc)." >&2 ;;
esac

echo "==> Quava $TAG installed. Log out and back in for uinput group membership to take effect."
