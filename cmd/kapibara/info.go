package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// infoCmd shows deploy-relevant server settings so users (and AI agents) know
// the default apps domain and registry host without asking.
func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show server info: apps domain, registry host, public URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			var out struct {
				AppsDomain   string `json:"appsDomain"`
				RegistryHost string `json:"registryHost"`
				PublicURL    string `json:"publicUrl"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/config", nil, &out); err != nil {
				return err
			}
			fmt.Printf("server:        %s\n", cfg.Server)
			fmt.Printf("apps domain:   %s\n", orDash(out.AppsDomain))
			if out.AppsDomain != "" {
				fmt.Printf("  → apps are reachable at <app>.%s\n", out.AppsDomain)
			}
			fmt.Printf("registry host: %s\n", orDash(out.RegistryHost))
			fmt.Printf("public url:    %s\n", orDash(out.PublicURL))
			return nil
		},
	}
}

func orDash(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
