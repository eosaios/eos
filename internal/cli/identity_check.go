package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/eosaios/eos/internal/config"
	"github.com/eosaios/eos/internal/i18n"
	"github.com/eosaios/eos/internal/identitypilot"
	"github.com/eosaios/eos/pkg/executil"
	"github.com/spf13/cobra"
)

func newIdentityCheckCmd() *cobra.Command {
	var settings identitypilot.Settings
	command := &cobra.Command{
		Use:    "identity-check",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _ := config.Load()
			settings.Confirmation = i18n.T("identity.check.browser_received", cfg.Language)
			id, err := identitypilot.Check(cmd.Context(), settings, openIdentityBrowser)
			if err != nil {
				return errors.New(i18n.T(err.Error(), cfg.Language))
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), id)
			return err
		},
	}
	command.Flags().StringVar(&settings.Issuer, "issuer", "", "OIDC issuer")
	command.Flags().StringVar(&settings.APIOrigin, "api-origin", "", "Account API origin")
	command.Flags().StringVar(&settings.ClientID, "client-id", "", "Registered public client ID")
	command.Flags().StringVar(&settings.CAFile, "loopback-ca-file", "", "CA bundle for isolated loopback HTTPS testing")
	return command
}

func openIdentityBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return executil.StartDetached(exec.Command("open", target))
	case "windows":
		return executil.StartDetached(exec.Command("rundll32", "url.dll,FileProtocolHandler", target))
	default:
		return executil.StartDetached(exec.Command("xdg-open", target))
	}
}
