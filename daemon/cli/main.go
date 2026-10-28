package main

import (
	"quava-cli/commands"
)

func init() {
	commands.Root.AddCommand(
		commands.Connect,
		commands.Disconnect,
		commands.Discover,
		commands.Ping,
		commands.Devices,
		commands.Pair,
		commands.Ring,
		commands.Clipboard,
		commands.Notification,
		commands.Input,
		commands.Media,
	)
}

func main() {
	commands.Execute()
}
