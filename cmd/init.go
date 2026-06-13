package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/config"
	"github.com/vdpeijl/clk/internal/gitctx"
	"github.com/vdpeijl/clk/internal/hookinstall"
	"github.com/vdpeijl/clk/internal/store"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Install tool hooks and register the current project",
	Long: `Detects the dev tools in use (Claude Code, Cursor, Copilot, git),
installs their capture hooks, registers the repository so the daemon watches it
for file activity, and scaffolds a committed .clk.toml carrying the project
mapping and description template so teammates inherit the conventions on clone.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runInit(cmd)
	},
}

func runInit(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine working directory: %w", err)
	}

	// Prefer the repository root so hooks and the registry key off the same path
	// regardless of which subdirectory `clk init` is run from.
	root := gitctx.Root(cwd)
	if root == "" {
		root = cwd
	}
	token := gitctx.Detect(root).ProjectToken

	fmt.Fprintf(out, "Initializing clk in %s\n", root)

	if err := installHooks(out, root); err != nil {
		return err
	}
	if err := registerProject(root, token); err != nil {
		return fmt.Errorf("register project: %w", err)
	}
	fmt.Fprintf(out, "  registered project %q for file-watch capture\n", token)

	created, err := config.ScaffoldProject(root, config.ProjectConfig{Template: config.DefaultTemplate})
	if err != nil {
		return fmt.Errorf("scaffold .clk.toml: %w", err)
	}
	if created {
		fmt.Fprintln(out, "  created .clk.toml (commit it so teammates inherit the mapping)")
	} else {
		fmt.Fprintln(out, "  .clk.toml already present — left unchanged")
	}

	fmt.Fprintln(out, "Done.")
	return nil
}

// installHooks detects the tools in use under root and installs each one's
// capture hook, reporting progress to out.
func installHooks(out io.Writer, root string) error {
	tools := hookinstall.Detect(detection(root))
	if len(tools) == 0 {
		fmt.Fprintln(out, "  no supported dev tools detected — only file-watch capture will run")
		return nil
	}

	for _, tool := range tools {
		changed, err := installHook(root, tool)
		if err != nil {
			return fmt.Errorf("install %s hook: %w", tool, err)
		}
		if changed {
			fmt.Fprintf(out, "  installed %s hook\n", tool)
		} else {
			fmt.Fprintf(out, "  %s hook already installed\n", tool)
		}
	}
	return nil
}

// detection probes the filesystem and PATH to decide which tools are in use.
func detection(root string) hookinstall.Detection {
	return hookinstall.Detection{
		ClaudeDir:  dirExists(filepath.Join(root, ".claude")),
		ClaudeBin:  binOnPath("claude"),
		CursorDir:  dirExists(filepath.Join(root, ".cursor")),
		CursorBin:  binOnPath("cursor"),
		CopilotDir: dirExists(filepath.Join(root, ".copilot")),
		CopilotBin: binOnPath("copilot"),
		GitRepo:    dirExists(filepath.Join(root, ".git")),
	}
}

// installHook installs the capture hook for a single tool, returning whether it
// changed anything on disk.
func installHook(root string, tool hookinstall.Tool) (bool, error) {
	switch tool {
	case hookinstall.ToolClaudeCode:
		return installJSONHook(filepath.Join(root, ".claude", "settings.json"), func(existing []byte) ([]byte, bool, error) {
			return hookinstall.MergeClaudeSettings(existing, hookinstall.ClaudeCommand)
		})
	case hookinstall.ToolCursor:
		return installJSONHook(filepath.Join(root, ".cursor", "hooks.json"), func(existing []byte) ([]byte, bool, error) {
			return hookinstall.MergeEventHooks(existing, []string{"afterFileEdit", "beforeShellExecution"}, hookinstall.CursorCommand)
		})
	case hookinstall.ToolCopilot:
		return installJSONHook(filepath.Join(root, ".copilot", "hooks.json"), func(existing []byte) ([]byte, bool, error) {
			return hookinstall.MergeEventHooks(existing, []string{"postToolUse"}, hookinstall.CopilotCommand)
		})
	case hookinstall.ToolGit:
		return installGitHook(root)
	default:
		return false, fmt.Errorf("unknown tool %q", tool)
	}
}

// installJSONHook reads the JSON config at path, applies merge, and writes it
// back when merge reports a change. A missing file is treated as empty.
func installJSONHook(path string, merge func([]byte) ([]byte, bool, error)) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	out, changed, err := merge(existing)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// installGitHook installs the clk invocation into the repository's post-commit
// hook, preserving any existing script.
func installGitHook(root string) (bool, error) {
	path := filepath.Join(root, ".git", "hooks", "post-commit")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	out, changed := hookinstall.MergePostCommitHook(string(existing), hookinstall.GitCommand)
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(out), 0o755); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// registerProject records the repository in the local registry so the daemon
// file-watches it.
func registerProject(root, token string) error {
	path, err := dbPath()
	if err != nil {
		return err
	}
	st, err := store.Open(path)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.RegisterProject(root, token, time.Now())
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// binOnPath reports whether an executable named name is on PATH.
func binOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}
