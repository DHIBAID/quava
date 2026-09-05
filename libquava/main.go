package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"libquava/discovery"
	"libquava/pairing"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "discover":
		cmdDiscover(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "ping":
		cmdPing(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage:")
	fmt.Println("  go run . discover [--timeout 10s]")
	fmt.Println("  go run . pair <device-name> [--timeout 120s]")
	fmt.Println("  go run . ping <device-name> [--timeout 120s]")
}

func cmdDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 10*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	devices, err := discovery.Discover(ctx)
	if err != nil && ctx.Err() == nil {
		fmt.Printf("discover failed: %v\n", err)
		os.Exit(1)
	}

	if len(devices) == 0 {
		fmt.Println("no Quava devices discovered")
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(devices); err != nil {
		fmt.Printf("failed to print devices: %v\n", err)
		os.Exit(1)
	}
}

func cmdPair(args []string) {
	if len(args) == 0 {
		fmt.Println("missing device name")
		os.Exit(2)
	}

	deviceName := args[0]
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 120*time.Second, "overall timeout")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client, err := discovery.NewClient()
	if err != nil {
		fmt.Printf("failed to create discovery client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	device, err := client.Find(ctx, deviceName)
	if err != nil {
		fmt.Printf("discover failed: %v\n", err)
		os.Exit(1)
	}

	conn, err := device.Connect(ctx)
	if err != nil {
		fmt.Printf("connect failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)
	result, err := pairing.Initiate(ctx, conn.ProtocolConn(), pairing.InitiatorOptions{
		DeviceName:   localDeviceName(),
		Capabilities: []uint64{},
	}, func(code uint32, remoteName string) (bool, error) {
		fmt.Printf("Pairing code for %s: %03d %03d\n", remoteName, code/1000, code%1000)
		fmt.Print("Confirm pairing? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		return line == "y" || line == "yes", nil
	})
	if err != nil {
		fmt.Printf("pair failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Paired with %s using device %s\n", device.Name, result.PeerDeviceID)
}

func cmdPing(args []string) {
	if len(args) == 0 {
		fmt.Println("missing device name")
		os.Exit(2)
	}

	deviceName := args[0]
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 120*time.Second, "overall timeout")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client, err := discovery.NewClient()
	if err != nil {
		fmt.Printf("failed to create discovery client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	device, err := client.Find(ctx, deviceName)
	if err != nil {
		fmt.Printf("discover failed: %v\n", err)
		os.Exit(1)
	}

	conn, err := device.Connect(ctx)
	if err != nil {
		fmt.Printf("connect failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)
	if _, err := pairing.Initiate(ctx, conn.ProtocolConn(), pairing.InitiatorOptions{DeviceName: localDeviceName()}, func(code uint32, remoteName string) (bool, error) {
		fmt.Printf("Pairing code for %s: %03d %03d\n", remoteName, code/1000, code%1000)
		fmt.Print("Confirm pairing? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		return line == "y" || line == "yes", nil
	}); err != nil {
		fmt.Printf("pair failed: %v\n", err)
		os.Exit(1)
	}

	if err := conn.Ping(ctx); err != nil {
		fmt.Printf("ping failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Connected to %s\nPAIR -> PING -> PONG\n", device.Name)
}

func localDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "Quava"
	}
	return hostname
}
