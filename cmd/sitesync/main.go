package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/carlosrgl/sitesync/internal/config"
	"github.com/carlosrgl/sitesync/internal/logger"
	syncsvc "github.com/carlosrgl/sitesync/internal/sync"
	"github.com/carlosrgl/sitesync/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "3.0.0-dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	flagConf  string
	flagNoTUI bool
	flagDry   bool
)

var rootCmd = &cobra.Command{
	Use:   "sitesync [sql|files]",
	Short: "Sync a remote website to your local environment",
	Long: `sitesync synchronises a remote website (database + files) to your local
development environment. Running without arguments opens the interactive TUI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opStr := ""
		if len(args) > 0 {
			opStr = args[0]
		}
		op := tui.ParseOp(opStr)

		if flagNoTUI || flagConf != "" && len(args) > 0 {
			return runHeadless(flagConf, op)
		}

		return runTUI(flagConf)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("sitesync", version)
	},
}

var replaceCmd = &cobra.Command{
	Use:   "replace <search> <replace> <file>",
	Short: "PHP serialize-aware find/replace on a file",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return syncsvc.ResilientReplaceFile(args[0], args[1], args[2], syncsvc.ReplaceOptions{})
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate shell config files to TOML format",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")

		if all {
			entries, err := config.ListShellConfigs()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No shell configs found to migrate.")
				return nil
			}
			for _, name := range entries {
				if err := migrateOne(name, flagDry); err != nil {
					fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
				}
			}
			return nil
		}

		if flagConf == "" {
			return fmt.Errorf("specify --conf=NAME or --all")
		}
		return migrateOne(flagConf, flagDry)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConf, "conf", "", "Config name (etc/{name}/config.toml)")
	rootCmd.PersistentFlags().BoolVar(&flagNoTUI, "no-tui", false, "Run headlessly (no interactive interface)")

	migrateCmd.Flags().Bool("all", false, "Migrate all shell configs found in etc/")
	migrateCmd.Flags().BoolVar(&flagDry, "dry-run", false, "Preview migration without writing files")

	rootCmd.AddCommand(versionCmd, replaceCmd, migrateCmd)
}

// ── TUI runner ───────────────────────────────────────────────────────────────

func runTUI(preselect string) error {
	entries, err := config.ListConfigs()
	if err != nil {
		return fmt.Errorf("listing configs: %w", err)
	}

	log := logger.Discard()
	m := tui.New(entries, preselect, log)

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// ── headless runner ──────────────────────────────────────────────────────────

func runHeadless(confName string, op syncsvc.Op) error {
	if confName == "" {
		return fmt.Errorf("--conf is required for headless mode")
	}

	cfg, err := config.Load(confName)
	if err != nil {
		return err
	}

	logFile := config.LogFile(cfg)
	log, err := logger.New(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot open log file: %v\n", err)
		log = logger.Discard()
	}
	defer log.Close()

	return syncsvc.RunHeadless(nil, cfg, op, log)
}

// ── migrate helper ───────────────────────────────────────────────────────────

func migrateOne(name string, dryRun bool) error {
	result, err := config.MigrateShellConfig(name)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Printf("=== DRY RUN: %s ===\n%s\n", name, result.Preview)
		return nil
	}
	if err := config.Save(name, result.Config); err != nil {
		return err
	}
	fmt.Printf("Migrated %s → etc/%s/config.toml (%d fields)\n",
		name, name, result.FieldCount)
	if len(result.Unknown) > 0 {
		fmt.Println("  Unmapped variables (review manually):")
		for _, u := range result.Unknown {
			fmt.Println("    " + u)
		}
	}
	return nil
}
