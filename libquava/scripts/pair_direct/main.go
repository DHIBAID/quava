
package main

import (
    "bufio"
    "context"
    "flag"
    "fmt"
    "log"
    "net"
    "os"
    "strings"
    "time"

    "libquava/pairing"
    "libquava/protocol"
)

func main() {
    addr := flag.String("addr", "127.0.0.1:48273", "target address host:port")
    timeout := flag.Duration("timeout", 120*time.Second, "overall timeout")
    flag.Parse()

    ctx, cancel := context.WithTimeout(context.Background(), *timeout)
    defer cancel()

    netConn, err := net.Dial("tcp4", *addr)
    if err != nil {
        log.Fatalf("dial %s: %v", *addr, err)
    }
    defer netConn.Close()

    conn := protocol.NewConn(netConn)

    reader := bufio.NewReader(os.Stdin)
    result, err := pairing.Initiate(ctx, conn, pairing.InitiatorOptions{DeviceName: "local-test"}, func(code uint32, remoteName string) (bool, error) {
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
        log.Fatalf("pair failed: %v", err)
    }

    fmt.Printf("Paired with %s using device %s\n", result.PeerDeviceName, result.PeerDeviceID)
}
