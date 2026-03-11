package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/uptimy/uptimy-agent/internal/config"
	"github.com/uptimy/uptimy-agent/internal/runtime"
	"github.com/uptimy/uptimy-agent/internal/version"
	"gopkg.in/yaml.v3"
)

var (
	configPath string
	logLevel   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "uptimy-agent",
		Short: "Uptimy Agent — self-healing infrastructure agent",
		Long: `Uptimy Agent is an open-source self-healing agent that detects
infrastructure issues and applies deterministic reparations.`,
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Override log level (debug, info, warn, error)")

	rootCmd.AddCommand(
		newRunCmd(),
		newVersionCmd(),
		newInitCmd(),
		newValidateCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start the agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadFromFile(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Override log level from flag if specified.
			if logLevel != "" {
				cfg.Logging.Level = logLevel
			}

			rt, err := runtime.New(cfg)
			if err != nil {
				return fmt.Errorf("initialising runtime: %w", err)
			}

			// Set up signal handling for graceful shutdown.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				sig := <-sigCh
				fmt.Fprintf(os.Stderr, "\nReceived signal %s, shutting down...\n", sig)
				cancel()
			}()

			// Run the agent (blocks until context is cancelled or error).
			errCh := make(chan error, 1)
			go func() {
				errCh <- rt.Start(ctx)
			}()

			// Wait for either the agent to stop or a signal.
			select {
			case err := <-errCh:
				if err != nil {
					return fmt.Errorf("agent error: %w", err)
				}
			case <-ctx.Done():
			}

			// Graceful shutdown with timeout.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			if err := rt.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("shutdown error: %w", err)
			}

			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("uptimy-agent %s\n", version.Version)
			fmt.Printf("  commit: %s\n", version.Commit)
			fmt.Printf("  built:  %s\n", version.BuildDate)
		},
	}
}

func newInitCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate an example configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshalling config: %w", err)
			}

			if output != "" {
				if err := os.WriteFile(output, data, 0644); err != nil {
					return fmt.Errorf("writing config to %s: %w", output, err)
				}
				fmt.Printf("Configuration written to %s\n", output)
			} else {
				fmt.Print(string(data))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Write config to file instead of stdout")
	return cmd
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate a configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.LoadFromFile(configPath)
			if err != nil {
				return fmt.Errorf("configuration invalid: %w", err)
			}
			fmt.Println("Configuration is valid.")
			return nil
		},
	}
}
