// Package executil owns the lifecycle of short-lived host launcher processes.
package executil

import (
	"log/slog"
	"os/exec"
)

// StartDetached returns once the command starts, then reaps it asynchronously.
// It preserves the command's arguments, environment and process attributes;
// it does not create an OS session or cancel the child with a UI request.
// The caller transfers ownership of Wait and must not call it again.
func StartDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("host.launcher_exited", "error", err)
		}
	}()
	return nil
}
