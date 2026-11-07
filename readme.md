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
binaries (`amd64`/`arm64` tarballs), a `.deb`/`.rpm` for the desktop app, and a signed
Android APK. See `.github/workflows/release.yml`.

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

## Using Quava

1. Open Quava on the phone and keep it running.
2. On the computer, open the desktop app, choose **Devices > Find devices**, pick the
   phone and **Pair**. Check that the code matches on both screens.
3. After that the phone connects by itself, whenever it is nearby (see below).

While connected, the **Devices** page shows how the phone is doing: battery
(and whether it is charging), Wi-Fi and mobile signal with 4G/5G, Bluetooth and free
storage. The mobile network type needs the phone-state permission, which the Android
app asks for once; without it the computer just shows "Mobile". Per-phone options
(automatic connection, notification direction, clipboard sync, remote input, ping and
ring, unpair) are on the **Settings** page.

The same things are available from the command line: `quava --help`. Useful ones:
`quava status`, `quava unpair <id>`, `quava autoconnect on|off <id>`,
`quava media ...`, `quava audio ...`, `quava notification filters ...`.

### How it reconnects

The daemon connects to every paired phone when it starts, and whenever a phone
appears on the network. It does not keep scanning: it listens for the phone's
mDNS announcement (the same idea KDE Connect uses), and only asks when a paired
phone is missing, once at first, then backing off to about once a minute. It also
asks once when you move to a different network. Turn this off per phone with the
"Connect automatically" switch or `quava autoconnect off <id>`.

### Settings files

The daemon keeps everything in `~/.config/quava/config.yaml`: identity, paired
phones, per-phone settings, and notification filters. Per-phone settings are in the
desktop app's Settings page or the CLI. Which notifications get sent is chosen on the
phone (Notification settings); the host-side filters are CLI only
(`quava notification filters`).

## Security

- **Pairing** uses an X25519 key exchange. You confirm a six-digit code on both
  screens, which stops anyone in the middle from slipping in.
- **Sessions** start with a mutual check of both devices' identities using the
  secret created at pairing. Then **every message in both directions is encrypted
  and authenticated** with ChaCha20-Poly1305. Keys are new for each session (they
  come from the pairing secret plus fresh random values), each direction has its
  own key, and a per-direction counter makes replayed, reordered or reflected
  messages fail. There is no unencrypted fallback.
- This is protocol version 2. Phone and computer must both be updated; a mismatched
  pair refuses to connect and says so in the log.

## Development

```sh
cd daemon
make lint       # go vet, go fmt, golangci-lint, staticcheck, deadcode
```

`make lint` needs `golangci-lint`, `staticcheck` and `deadcode` on your `PATH`
(`go install honnef.co/go/tools/cmd/staticcheck@latest`,
`go install golang.org/x/tools/cmd/deadcode@latest`). The extra linters live in
`daemon/.golangci.yml`; they check error handling (`errors.Is`/`As`, `%w`),
needless `fmt`, and shadowed built-ins. The desktop app builds with strict
warnings (`-Wshadow -Wconversion -Wcast-qual ...`) and must stay warning-free.

### Error style

One style everywhere, so an error reads the same in the log, the CLI and the apps.

| Rule | Example |
| --- | --- |
| Lowercase, no trailing period, one line | `cannot reach quavad: connection refused` |
| Name the component, then the step that failed, then the cause: `component: step: cause` | `session: send ping: write tcp: broken pipe` |
| Wrap the cause with `%w` (Go) or the exception cause (Kotlin) so callers can inspect it | `fmt.Errorf("config: save: %w", err)` |
| A message about bad input that is shown as-is may skip the component | `volume must be between 0.0 and 1.0` |
| Never print an error and also return it; the layer that finally shows it does that, once | |

Components in Go are the package or subsystem: `session`, `config`, `protocol`,
`discovery`, `crypto`, `pairing`, `mpris`, `audio`, `artwork`, `uinput`,
`remote input`, `clipboard`, `media`, `profiling`.

Where the message ends up:

- **The daemon** answers a failed request with `{"error": "<message>"}`, same style, no prefix added.
- **The CLI** prints `quava: <message>` on standard error and exits with status 1. Nothing else prints errors.
- **The desktop app** shows the message in a toast, with the first letter capitalized.
- **The Android app** logs the exception (lowercase) and shows status text as a sentence, e.g. `Connection failed: <message>`.
- **Logs** read `component: what happened: cause`, for example `session: phone status: battery=100% ...`.


Notes:
1. Need to replace Zenity with custom UI because Zenity is not a universal GTK package, or we can consider it to be a requirement for using the service?
Need to see how the alternatives will work before we replace it in `libquava/notifications/android.go:promptAndReply`

2. Current linux UI implementation uses Polling in many cases, it's better to modify daemon to be able to push events through the socket to update daemon state. But I'm too fried to do that right now (we love GTK)


# Feature list
- [x] Ping 
- [x] Ring
- [x] Unpair
- [x] Device status (battery, signal, Wi-Fi, Bluetooth, storage)
- [x] Auto-connect to paired devices
- [x] Clipboard Sync (A->L)
- [x] Notification Sync
- [x] Remote Input (A->L)
- [x] Multimedia Controls
- [ ] Pre-saved command running
- [ ] SFTP/SMB
- [x] SSH
- [ ] Exit Node/VPN
- [ ] Screenshare/RDP
- [ ] Password Manager(?)
- [ ] 2FA code sync(?)
- [ ] SMS Interaction