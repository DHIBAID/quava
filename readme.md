# Quava

Connect an Android phone to a Linux computer: notifications, clipboard, media
controls, remote input (touchpad and keyboard), ping/ring, and system volume.

| Part | What it is | Where |
| --- | --- | --- |
| `quavad` | Background daemon (Go). Talks to the phone, owns all state. | `daemon/quavad` |
| `quava` | Command-line tool for the daemon (Go). | `daemon/cli` |
| Quava (desktop) | GTK4 app (C). Optional, talks to the daemon. | `linux/` |
| Quava (Android) | Phone app (Kotlin, Jetpack Compose). | `android/` |

## Releases

Tagged releases (`vX.Y.Z`, [SemVer](https://semver.org/)) are built automatically and
published to [GitHub Releases](https://github.com/DHIBAID/quava/releases): daemon/CLI
binaries (`amd64`/`arm64` tarballs), a `.deb`/`.rpm`/AppImage for the desktop app, and a
signed Android APK. See `.github/workflows/release.yml`.

## Install on Linux

### What you need

Runtime (daemon):

| Needs | Why | Fedora package |
| --- | --- | --- |
| systemd user session, D-Bus session bus | runs the daemon, notifications, media players | `systemd`, `dbus` |
| `/dev/uinput` access | remote input (see `make uinput`) | kernel module `uinput` |
| PipeWire (`wpctl`) **or** PulseAudio (`pactl`) | system volume and output devices; picked automatically | `wireplumber` or `pulseaudio-utils` |
| An MPRIS media player | media page (Spotify, Firefox, VLC, mpv, ...) | |
| `zenity` | reply prompt for notification actions | `zenity` |
| `rsvg-convert` or ImageMagick (optional) | shows SVG-only app icons next to media players | `librsvg2-tools` or `ImageMagick` |

Build:

| Needs | Why | Fedora package |
| --- | --- | --- |
| Go 1.27 or newer | `go.mod` requires it | `golang` |
| `make` | install and lint targets | `make` |

### Daemon and CLI

```sh
cd daemon
make systemd        # builds quavad + quava, installs a socket-activated user service
```

`make systemd` also sets up `uinput` access (it asks for sudo, and you need to log
out and back in once). `make update-systemd` rebuilds and restarts after code
changes. `make clean-systemd` removes everything, including the config.

Look at the daemon's logs:

```sh
journalctl --user -u quavad.service -f -n 50
```

The user systemd unit needs the graphical-session environment. If remote input says
no desktop session is available, run this from your desktop session and restart the
service:

```sh
systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XAUTHORITY XDG_RUNTIME_DIR XDG_SESSION_TYPE
systemctl --user restart quavad.service
```

### Desktop app

| Needs | Fedora package |
| --- | --- |
| GTK 4 (4.20 recommended; follows the system dark/light setting) | `gtk4-devel` |
| JSON-GLib | `json-glib-devel` |
| `glib-compile-resources` (bundles the stylesheet and icon) | `glib2-devel` |
| GCC, make, pkg-config | `gcc make pkgconf-pkg-config` |

```sh
cd linux
make run        # build and launch
make install    # add Quava to your app menu (installs quava-gui and its icon)
```

The app also registers its own launcher entry and icon on every start, which is
what makes the icon show up in the dock on Wayland.

## Install on Android

| Needs | Version |
| --- | --- |
| Android Studio or the Android SDK | compile/target SDK 37, min SDK 24 (Android 7.0) |
| JDK | 17 or newer (the Makefile assumes `/usr/lib/jvm/java-21-openjdk`; change `JDK` there) |
| Gradle | 9.6 (the wrapper `./gradlew` downloads it) |
| ADB | for installing over USB or Wi-Fi |

Libraries (fetched by Gradle): Kotlin 2.2.10, Android Gradle Plugin 9.4.1, Jetpack
Compose BOM 2026.02.01 with Material 3 and extended icons, Coil 2.7.0, AndroidX Media
1.7.0, Bouncy Castle 1.78.1, CBOR 4.5.2.

```sh
cd android
make run        # build, install and launch on a connected phone
```

Grant notification access when the app asks, so the phone can forward notifications.

## Development

```sh
cd daemon
make lint       # go vet, go fmt, golangci-lint, staticcheck, deadcode
```

`make lint` needs `golangci-lint`, `staticcheck` and `deadcode` on your `PATH`
(`go install honnef.co/go/tools/cmd/staticcheck@latest`,
`go install golang.org/x/tools/cmd/deadcode@latest`). The desktop app builds with
strict warnings (`-Wshadow -Wconversion -Wcast-qual ...`) and must stay warning-free.

Error-message style: `docs/error-style.md`. See `docs/` for everything else
(usage, protocol/security details, feature status).