package ssh

import (
	"context"
	"strings"
	"testing"
)

// Error messages follow docs/error-style.md: a lowercase component prefix,
// then the failed step, then the cause.
func TestStartCanceledErrorStyle(t *testing.T) {
	service := NewSSHService("", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.Start(ctx)
	if err == nil {
		t.Fatal("Start with a canceled context returned no error")
	}
	if got := err.Error(); !strings.HasPrefix(got, "ssh: ") {
		t.Fatalf("error %q does not start with the ssh component prefix", got)
	}
}
