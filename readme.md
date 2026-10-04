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

## Linux Pre-requisites
1. ???


Notes:
1. Need to replace Zenity with custom UI because Zenity is not a universal GTK package, or we can consider it to be a requirement for using the service?
Need to see how the alternatives will work before we replace it in `libquava/notifications/android.go:promptAndReply`