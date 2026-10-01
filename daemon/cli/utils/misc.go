package utils

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func Fail(message string) {
	fmt.Fprintln(os.Stderr, "quava:", message)
	os.Exit(1)
}

func Usage() {
	fmt.Println("usage:\n  quava discover [--timeout 10s]")
	fmt.Println("          quava devices [peer_device_id]")
	fmt.Println("          quava pair <peer_device_id> [--timeout 2m]")
	fmt.Println("          quava connect <peer_device_id>")
	fmt.Println("          quava ping <peer_device_id> [--timeout 15s]")
}

func ParseTimeout(name string, args []string, fallback time.Duration) (time.Duration, []string) {
	timeout := fallback
	positionals := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]

		if argument == "--timeout" {
			if index+1 == len(args) {
				fmt.Fprintf(os.Stderr, "%s: --timeout needs a value\n", name)
				os.Exit(2)
			}
			index++

			parsed, err := time.ParseDuration(args[index])
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: invalid timeout: %v\n", name, err)
				os.Exit(2)
			}
			timeout = parsed
			continue
		}
		if strings.HasPrefix(argument, "--timeout=") {
			parsed, err := time.ParseDuration(strings.TrimPrefix(argument, "--timeout="))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: invalid timeout: %v\n", name, err)
				os.Exit(2)
			}

			timeout = parsed
			continue
		}
		positionals = append(positionals, argument)
	}

	return timeout, positionals
}
