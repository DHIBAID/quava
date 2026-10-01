package main

import (
	"os"

	"quava-cli/commands"
	"quava-cli/utils"
)

func main() {
	// go run . <command> [args]
	// would like quava-cli <command> [args] 
	if len(os.Args) < 2 {
		utils.Usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "discover":
		commands.Discover(os.Args[2:])

	case "devices":
		commands.Devices(os.Args[2:])

	case "pair":
		commands.Pair(os.Args[2:])

	case "connect":
		commands.Connect(os.Args[2:])

	case "ping":
		commands.Ping(os.Args[2:])

	default:
		utils.Usage()
		os.Exit(2)
	}
}
