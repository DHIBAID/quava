# Quava

## Android Pre-requisites
1. Android SDK
2. Java
3. Gradle (included with Android Studio)
4. Kotlin
5. ADB (wired or wireless)
6. Make

## Daemon Pre-requisites
1. Golang (needed for building the daemon)
2. Make
3. Systemd
4. Zenity (gtk)*
5. `/dev/uinput` access is required. Check makefile for confirguration (requires session refresh)

### Setup daemon:
```sh
make systemd
``` 

### Use CLI:
```
quava <command> [options] [--flags]
```

### Look at daemon logs:
```sh
journalctl --user -u quavad.service -f -n 50
```

The user systemd unit imports graphical-session variables before startup. If
remote input reports that no desktop session is available, run
`systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XAUTHORITY XDG_RUNTIME_DIR XDG_SESSION_TYPE`
from that desktop session, then restart `quavad.service`.

## Linux Pre-requisites
1. GTK4 (devel)
2. GCC
3. pkgconf-pkg-config
4. JSON-GLib (devel)

On Fedora, the build dependencies are `gtk4-devel json-glib-devel gcc make pkgconf-pkg-config`.

The daemon needs a running user systemd instance and a D-Bus session. Notification
reply actions additionally use `zenity`. If Connect only says it was requested,
inspect `journalctl --user -u quavad.service -f`: the session is started
asynchronously and discovery/network failures are logged there.


Notes:
1. Need to replace Zenity with custom UI because Zenity is not a universal GTK package, or we can consider it to be a requirement for using the service?
Need to see how the alternatives will work before we replace it in `libquava/notifications/android.go:promptAndReply`

2. Current linux UI implementation uses Polling in many cases, it's better to modify daemon to be able to push events through the socket to update daemon state. But I'm too fried to do that right now (we love GTK)
