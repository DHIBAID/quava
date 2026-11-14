package commands

// Commands that apply one daemon command to one paired device.
var (
	Connect = peerCommand("connect", "Connect to a paired device.",
		"Connect to a paired device now. The daemon also connects on its own; see autoconnect.",
		peerAction{"connect", "Connect requested for %s"})

	Disconnect = peerCommand("disconnect", "Disconnect from a paired device.", "",
		peerAction{"disconnect", "Disconnect requested for %s"})

	Unpair = peerCommand("unpair", "Forget a paired device.",
		`Forget a paired device. If it is connected, it is told to forget this computer
too; otherwise only this computer forgets it. Pair again to reconnect.`,
		peerAction{"unpair", "Unpaired %s"})

	Ping = peerCommand("ping", "Ping a connected device.", "",
		peerAction{"ping", "Ping sent to %s"})

	Ring = peerCommand("ring", "Make a connected device ring.", "",
		peerAction{"ring", "Ring requested for %s"})
)

// Commands that choose between several daemon commands for one device.
var (
	Clipboard = actionCommand("clipboard", "Control clipboard sharing with a device.",
		`push        send the current clipboard now
start       start watching the clipboard
stop        stop watching the clipboard
sync-on     share copied text automatically
sync-off    stop sharing copied text`,
		map[string]peerAction{
			"push":     {"clipboard-push", "Clipboard sent to %s"},
			"start":    {"clipboard-service-start", "Clipboard watching started for %s"},
			"stop":     {"clipboard-service-stop", "Clipboard watching stopped for %s"},
			"sync-on":  {"clipboard-sync-on", "Automatic clipboard sync on for %s"},
			"sync-off": {"clipboard-sync-off", "Automatic clipboard sync off for %s"},
		})

	Notification = actionCommand("notification", "Choose which way notifications travel.",
		`both       both directions
toHost     phone to computer (the default)
toClient   computer to phone
none       no notification sync`,
		map[string]peerAction{
			"both":     {"notif-sync-both", "Notification sync set to both ways for %s"},
			"toHost":   {"notif-sync-tohost", "Notification sync set to phone to computer for %s"},
			"toClient": {"notif-sync-toclient", "Notification sync set to computer to phone for %s"},
			"none":     {"notif-sync-stop", "Notification sync turned off for %s"},
		})

	Input = actionCommand("input", "Allow or block remote input from a device.",
		`enable      turn remote input on for the current connection
disable     turn it off
auto-on     turn it on automatically every time the device connects
auto-off    stop doing that`,
		map[string]peerAction{
			"enable":   {"input-enable", "Remote input enabled for %s"},
			"disable":  {"input-disable", "Remote input disabled for %s"},
			"auto-on":  {"input-auto-on", "Remote input will turn on automatically for %s"},
			"auto-off": {"input-auto-off", "Remote input no longer turns on automatically for %s"},
		})

	AutoConnect = actionCommand("autoconnect", "Connect to a device automatically.",
		`on     connect when the daemon starts and whenever the device appears (the default)
off    don't; an existing connection is left alone`,
		map[string]peerAction{
			"on":  {"autoconnect-on", "Auto-connect on for %s"},
			"off": {"autoconnect-off", "Auto-connect off for %s"},
		})

	Ssh = actionCommand("ssh", "Allow or block SSH tunnel access from a device.",
		`enable      allow this device to open an SSH terminal tunnel
disable     block it (the default for newly paired devices)`,
		map[string]peerAction{
			"enable":  {"ssh-enable", "SSH tunnel access enabled for %s"},
			"disable": {"ssh-disable", "SSH tunnel access disabled for %s"},
		})
)
