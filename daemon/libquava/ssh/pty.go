package ssh

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

type ptyRequest struct {
	Term   string
	Cols   uint32
	Rows   uint32
	Width  uint32
	Height uint32
	Modes  string
}

type windowChangeRequest struct {
	Cols   uint32
	Rows   uint32
	Width  uint32
	Height uint32
}

// runPTYShell starts an interactive login shell attached to a PTY.
func (s *SSHService) runPTYShell(channel ssh.Channel, requests <-chan *ssh.Request, initial ptyRequest) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-l")

	term := initial.Term
	if term == "" {
		term = "xterm"
	}

	cmd.Env = append(os.Environ(), "TERM="+term)

	ptmx, tty, err := pty.Open()
	if err != nil {
		_, _ = fmt.Fprintf(channel, "Failed to create PTY: %v\n", err)
		return
	}

	defer func() {
		_ = ptmx.Close()
		_ = tty.Close()
	}()

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	// Without this, the shell gets the pty as stdin/stdout/stderr but never
	// becomes a session leader with it as its controlling terminal. It still
	// looks like a tty (isatty() passes), but /dev/tty resolves to nothing,
	// so anything that opens /dev/tty directly - sudo for the password
	// prompt, among others - fails to find one.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := pty.Setsize(ptmx, &pty.Winsize{
		Cols: uint16(nonZeroDimension(initial.Cols)),
		Rows: uint16(nonZeroDimension(initial.Rows)),
		X:    uint16(initial.Width),
		Y:    uint16(initial.Height),
	}); err != nil {
		log.Printf("SSH PTY initial resize failed: %v", err)
	}

	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintf(channel, "Failed to start shell: %v\n", err)
		return
	}

	_ = tty.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	inputDone := make(chan struct{})
	processDone := make(chan error, 1)

	// PTY output -> SSH client.
	go func() {
		defer wg.Done()
		_, _ = io.Copy(channel, ptmx)
	}()

	// SSH client input -> PTY.
	go func() {
		defer wg.Done()
		defer close(inputDone)
		_, _ = io.Copy(ptmx, channel)
	}()

	// Handle terminal resize requests while the shell is running.
	go func() {
		for req := range requests {
			switch req.Type {
			case "window-change":
				var size windowChangeRequest
				if err := ssh.Unmarshal(req.Payload, &size); err != nil {
					continue
				}

				err := pty.Setsize(ptmx, &pty.Winsize{
					Cols: uint16(nonZeroDimension(size.Cols)),
					Rows: uint16(nonZeroDimension(size.Rows)),
					X:    uint16(size.Width),
					Y:    uint16(size.Height),
				})
				if err != nil {
					log.Printf("SSH PTY resize failed: %v", err)
				}

			default:
				_ = req.Reply(false, nil)
			}
		}
	}()

	go func() {
		processDone <- cmd.Wait()
	}()

	select {
	case err := <-processDone:
		if err != nil {
			log.Printf("ssh PTY shell exited: %v", err)
		}
	case <-inputDone:
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		if err := <-processDone; err != nil {
			log.Printf("ssh PTY shell exited after client disconnect: %v", err)
		}
	}

	_ = channel.Close()
	_ = ptmx.Close()
	wg.Wait()
}

func nonZeroDimension(value uint32) uint32 {
	if value == 0 {
		return 1
	}
	return value
}
