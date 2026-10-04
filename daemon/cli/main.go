package main

import (
	"quava-cli/commands"
)

func init() {
	commands.Root.AddCommand(
		commands.Connect,
		commands.Discover,
		commands.Ping,
		commands.Devices,
		commands.Pair,
		commands.Ring,
		commands.Clipboard,
	)
}

func main() {
	commands.Execute()
}
