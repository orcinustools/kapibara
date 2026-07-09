package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/orcinustools/kapibara/pkg/auth"
	"github.com/orcinustools/kapibara/pkg/config"
	"github.com/orcinustools/kapibara/pkg/store"
)

// adminCmd groups server-side administrative actions that operate directly on
// the control-plane store (run where the database is reachable, e.g. inside the
// kapibara pod/host).
func adminCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Server-side administrative actions"}

	var email, password string
	setPassword := &cobra.Command{
		Use:   "set-password",
		Short: "Reset a user's password (by email) directly in the store",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" || password == "" {
				return fmt.Errorf("--email and --password are required")
			}
			cfg := config.Load()
			st, err := store.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer st.Close()

			u, err := st.UserByEmail(email)
			if err != nil {
				return fmt.Errorf("user %q not found: %w", email, err)
			}
			hash, err := auth.HashPassword(password)
			if err != nil {
				return err
			}
			u.PasswordHash = hash
			// Clear 2FA so a reset restores access even if the authenticator was lost.
			u.TOTPSecret = ""
			u.TwoFAEnabled = false
			if err := st.UpdateUser(u); err != nil {
				return err
			}
			fmt.Printf("✓ password reset for %s (2FA disabled)\n", email)
			return nil
		},
	}
	setPassword.Flags().StringVar(&email, "email", "", "account email")
	setPassword.Flags().StringVar(&password, "password", "", "new password")

	cmd.AddCommand(setPassword)
	return cmd
}
