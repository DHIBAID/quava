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
	)
}

func main() {
	commands.Execute()
}
