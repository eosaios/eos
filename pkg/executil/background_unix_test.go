//go:build darwin || linux

package executil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func init() {
	if os.Getenv("EOS_BACKGROUND_CHILD") != "1" {
		return
	}
	var gate [1]byte
	if _, err := io.ReadFull(os.Stdin, gate[:]); err != nil {
		os.Exit(99)
	}
	if gate[0] == 'f' {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestStartDetachedReturnsBeforeExitAndReapsChild(t *testing.T) {
	for _, gateByte := range []byte{'s', 'f'} {
		t.Run(fmt.Sprintf("exit-%c", gateByte), func(t *testing.T) {
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "-test.run=^$")
			cmd.Env = append(os.Environ(), "EOS_BACKGROUND_CHILD=1")
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = input.Close()
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			})
			started := make(chan error, 1)
			go func() { started <- StartDetached(cmd) }()
			select {
			case err := <-started:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("launch waited for a child blocked on input")
			}
			pid := cmd.Process.Pid
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatalf("child should be alive before input: %v", err)
			}
			if _, err := input.Write([]byte{gateByte}); err != nil {
				t.Fatal(err)
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				err := syscall.Kill(pid, 0)
				if err == syscall.ESRCH {
					return
				}
				if err != nil {
					t.Fatalf("unexpected process probe: %v", err)
				}
				if time.Now().After(deadline) {
					t.Fatalf("owned child %d remains alive or unreaped after exit", pid)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestStartDetachedReportsStartFailure(t *testing.T) {
	cmd := exec.Command(filepath.Join(t.TempDir(), "missing-command"))
	if err := StartDetached(cmd); err == nil {
		t.Fatal("missing command accepted")
	}
	if cmd.Process != nil {
		t.Fatal("failed launch acquired a process")
	}
}
