package cli

import (
	"fmt"

	"github.com/Slothtron/mwt/internal/config"
	"github.com/Slothtron/mwt/internal/pathresolve"
)

// runSetupForRepo executes cfg.Setup against an already-resolved worktree context.
//
// This is the shared entry point for:
//   - `mwt add` (default, unless --no-setup)
//   - `mwt setup` (T07): call the same path after resolving the worktree path
//
// Empty / nil setup lists are a no-op (success).
func runSetupForRepo(runner SetupRunner, cfg *config.Config, ctx pathresolve.Context) error {
	if runner == nil {
		return fmt.Errorf("setup runner is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if err := runner.Run(ctx, cfg.Setup); err != nil {
		return err
	}
	return nil
}
