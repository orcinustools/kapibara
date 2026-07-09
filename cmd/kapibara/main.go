// Command kapibara is the control-plane server + CLI for the kapibara PaaS.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/orcinustools/kapibara/pkg/api"
	"github.com/orcinustools/kapibara/pkg/config"
	"github.com/orcinustools/kapibara/pkg/store"
	"github.com/orcinustools/kapibara/pkg/version"
)

func main() {
	root := &cobra.Command{
		Use:           "kapibara",
		Short:         "Kapibara — self-hosted PaaS on the orcinus cluster engine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(serveCmd(), migrateCmd(), versionCmd(), adminCmd())
	root.AddCommand(cliCommands()...)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the kapibara control-plane server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}

			st, err := store.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer st.Close()

			srv := api.New(cfg, st)
			httpSrv := &http.Server{
				Addr:              cfg.Addr,
				Handler:           srv.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			// Background scheduler for due backups.
			go srv.StartScheduler(ctx)
			// Background sampler accrues per-project metric history.
			go srv.StartMetricsSampler(ctx, 30*time.Second)

			go func() {
				fmt.Printf("kapibara %s listening on %s (engine: %s)\n",
					version.Version, cfg.Addr, cfg.OrcinusURL)
				if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					fmt.Fprintln(os.Stderr, "server error:", err)
					stop()
				}
			}()

			<-ctx.Done()
			fmt.Println("\nshutting down…")
			shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return httpSrv.Shutdown(shutCtx)
		},
	}
	return cmd
}

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Create/upgrade the control-plane database schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			st, err := store.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer st.Close()
			fmt.Println("migrations applied")
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kapibara %s (commit %s)\n", version.Version, version.GitCommit)
		},
	}
}
