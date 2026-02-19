package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/carlosrgl/sitesync/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive installer: set etc path, install binary, migrate configs",
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	cyan := "\033[36m"
	green := "\033[32m"
	yellow := "\033[33m"
	bold := "\033[1m"
	dim := "\033[2m"
	reset := "\033[0m"

	fmt.Println()
	fmt.Printf("%s%s⚡ sitesync setup%s\n", bold, cyan, reset)
	fmt.Printf("%s   Interactive installer for sitesync v%s%s\n\n", dim, version, reset)

	// ── Step 1: ETC path ────────────────────────────────────────────────────
	currentEtc := os.Getenv("SITESYNC_ETC")
	defaultEtc := currentEtc
	if defaultEtc == "" {
		home, _ := os.UserHomeDir()
		defaultEtc = filepath.Join(home, ".config", "sitesync")
	}

	fmt.Printf("%s1)%s Config directory (SITESYNC_ETC)\n", bold, reset)
	if currentEtc != "" {
		fmt.Printf("   Current: %s%s%s\n", green, currentEtc, reset)
	}
	fmt.Printf("   Path [%s]: ", defaultEtc)

	etcInput, _ := reader.ReadString('\n')
	etcInput = strings.TrimSpace(etcInput)
	etcPath := defaultEtc
	if etcInput != "" {
		etcPath = etcInput
	}

	// Expand ~ if used
	if strings.HasPrefix(etcPath, "~/") {
		home, _ := os.UserHomeDir()
		etcPath = filepath.Join(home, etcPath[2:])
	}

	// Make absolute
	if !filepath.IsAbs(etcPath) {
		cwd, _ := os.Getwd()
		etcPath = filepath.Join(cwd, etcPath)
	}

	// Create directory if needed
	if err := os.MkdirAll(etcPath, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", etcPath, err)
	}
	fmt.Printf("   %s✔%s Using %s\n\n", green, reset, etcPath)

	// ── Step 2: Shell profile ───────────────────────────────────────────────
	needsExport := currentEtc != etcPath
	shellFile := detectShellProfile()

	if needsExport && shellFile != "" {
		fmt.Printf("%s2)%s Shell environment\n", bold, reset)
		exportLine := fmt.Sprintf("export SITESYNC_ETC=%q", etcPath)

		// Check if already in shell file
		alreadySet := false
		if data, err := os.ReadFile(shellFile); err == nil {
			alreadySet = strings.Contains(string(data), "SITESYNC_ETC")
		}

		if alreadySet {
			fmt.Printf("   %s✔%s SITESYNC_ETC already set in %s\n", green, reset, filepath.Base(shellFile))
			fmt.Printf("   Updating to new value...\n")
			if err := updateShellExport(shellFile, "SITESYNC_ETC", exportLine); err != nil {
				fmt.Printf("   %s⚠%s  Could not update %s: %v\n", yellow, reset, shellFile, err)
				fmt.Printf("   Add manually: %s\n", exportLine)
			} else {
				fmt.Printf("   %s✔%s Updated %s\n", green, reset, filepath.Base(shellFile))
			}
		} else {
			fmt.Printf("   Add to %s? [Y/n]: ", filepath.Base(shellFile))
			ans, _ := reader.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			if ans == "" || ans == "y" || ans == "yes" {
				if err := appendToFile(shellFile, "\n# sitesync config directory\n"+exportLine+"\n"); err != nil {
					return fmt.Errorf("cannot write to %s: %w", shellFile, err)
				}
				fmt.Printf("   %s✔%s Added to %s\n", green, reset, filepath.Base(shellFile))
			} else {
				fmt.Printf("   %sSkipped.%s Add manually: %s\n", dim, reset, exportLine)
			}
		}

		// Set for current process so migration works
		os.Setenv("SITESYNC_ETC", etcPath)
		fmt.Println()
	} else if needsExport {
		fmt.Printf("%s2)%s Shell environment\n", bold, reset)
		fmt.Printf("   %s⚠%s  Could not detect shell profile.\n", yellow, reset)
		fmt.Printf("   Add this to your shell config:\n")
		fmt.Printf("   %sexport SITESYNC_ETC=%q%s\n\n", cyan, etcPath, reset)
		os.Setenv("SITESYNC_ETC", etcPath)
	} else {
		fmt.Printf("%s2)%s Shell environment\n", bold, reset)
		fmt.Printf("   %s✔%s SITESYNC_ETC already set correctly\n\n", green, reset)
	}

	// ── Step 3: Install binary ──────────────────────────────────────────────
	fmt.Printf("%s3)%s Install binary\n", bold, reset)

	binDir := defaultBinDir()
	selfPath, _ := os.Executable()

	fmt.Printf("   Install to [%s/sitesync]: ", binDir)
	binInput, _ := reader.ReadString('\n')
	binInput = strings.TrimSpace(binInput)
	if binInput != "" {
		binDir = binInput
	}
	if strings.HasPrefix(binDir, "~/") {
		home, _ := os.UserHomeDir()
		binDir = filepath.Join(home, binDir[2:])
	}

	destBin := filepath.Join(binDir, "sitesync")

	// Don't copy onto self
	if selfPath != "" {
		selfAbs, _ := filepath.Abs(selfPath)
		destAbs, _ := filepath.Abs(destBin)
		if selfAbs == destAbs {
			fmt.Printf("   %s✔%s Already installed at %s\n\n", green, reset, destBin)
		} else {
			if err := os.MkdirAll(binDir, 0755); err != nil {
				return fmt.Errorf("cannot create %s: %w", binDir, err)
			}
			data, err := os.ReadFile(selfPath)
			if err != nil {
				return fmt.Errorf("cannot read binary: %w", err)
			}
			if err := os.WriteFile(destBin, data, 0755); err != nil {
				return fmt.Errorf("cannot write binary: %w", err)
			}
			fmt.Printf("   %s✔%s Installed to %s\n\n", green, reset, destBin)
		}
	}

	// ── Step 4: Migrate configs ─────────────────────────────────────────────
	fmt.Printf("%s4)%s Migrate shell configs\n", bold, reset)

	shellConfigs, err := config.ListShellConfigs()
	if err != nil {
		fmt.Printf("   %s⚠%s  Cannot list configs: %v\n\n", yellow, reset, err)
	} else if len(shellConfigs) == 0 {
		fmt.Printf("   %s✔%s No shell configs to migrate (all TOML already)\n\n", green, reset)
	} else {
		fmt.Printf("   Found %d shell config(s) to migrate.\n", len(shellConfigs))
		fmt.Printf("   Migrate all? [Y/n]: ")
		ans, _ := reader.ReadString('\n')
		ans = strings.TrimSpace(strings.ToLower(ans))
		if ans == "" || ans == "y" || ans == "yes" {
			migrated := 0
			failed := 0
			for _, name := range shellConfigs {
				if err := migrateOne(name, false); err != nil {
					fmt.Printf("   %s✘%s %s: %v\n", "\033[31m", reset, name, err)
					failed++
				} else {
					migrated++
				}
			}
			fmt.Printf("   %s✔%s Migrated %d config(s)", green, reset, migrated)
			if failed > 0 {
				fmt.Printf(", %s%d failed%s", yellow, failed, reset)
			}
			fmt.Println()
		} else {
			fmt.Printf("   %sSkipped.%s Run `sitesync migrate --all` later.\n\n", dim, reset)
		}
	}

	// ── Summary ─────────────────────────────────────────────────────────────
	configs, _ := config.ListConfigs()
	fmt.Printf("%s%s── Setup complete ──%s\n", bold, green, reset)
	fmt.Printf("   Config dir:  %s\n", etcPath)
	fmt.Printf("   Binary:      %s\n", destBin)
	fmt.Printf("   Sites:       %d\n", len(configs))
	fmt.Println()
	if needsExport && shellFile != "" {
		fmt.Printf("   %s→ Restart your shell or run: source %s%s\n\n", yellow, filepath.Base(shellFile), reset)
	}

	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func detectShellProfile() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	shell := os.Getenv("SHELL")

	// zsh
	if strings.Contains(shell, "zsh") {
		for _, f := range []string{".zshrc", ".zprofile"} {
			p := filepath.Join(home, f)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// bash
	if strings.Contains(shell, "bash") {
		for _, f := range []string{".bashrc", ".bash_profile", ".profile"} {
			p := filepath.Join(home, f)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// fish
	if strings.Contains(shell, "fish") {
		p := filepath.Join(home, ".config", "fish", "config.fish")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Fallback: try common files
	for _, f := range []string{".zshrc", ".bashrc", ".profile"} {
		p := filepath.Join(home, f)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func defaultBinDir() string {
	home, _ := os.UserHomeDir()

	// Check if ~/bin or ~/.local/bin exists and is in PATH
	candidates := []string{
		filepath.Join(home, "bin"),
		filepath.Join(home, ".local", "bin"),
	}

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, c := range candidates {
		for _, p := range pathDirs {
			if p == c {
				return c
			}
		}
	}

	// macOS: check for Homebrew
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			return "/usr/local/bin"
		}
	}

	// Default to ~/bin
	return filepath.Join(home, "bin")
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func updateShellExport(path, varName, newLine string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export "+varName+"=") ||
			strings.HasPrefix(trimmed, varName+"=") {
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		return appendToFile(path, "\n# sitesync config directory\n"+newLine+"\n")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}
