package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/clockify"
	"github.com/vdpeijl/clk/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Clockify authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store your Clockify API key and select a workspace",
	Long: `Prompts for your Clockify API key, verifies it by listing your
workspaces, lets you pick one when you belong to more than one, and stores both
in ~/.clk/config.toml at mode 0600. The CLOCKIFY_API_KEY environment variable,
when set, overrides the stored key at runtime.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runAuthLogin(cmd)
	},
}

func runAuthLogin(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())

	apiKey, err := promptAPIKey(out, in)
	if err != nil {
		return err
	}
	if apiKey == "" {
		return fmt.Errorf("no API key entered")
	}

	client := clockify.New(apiKey, "")
	workspaces, err := client.Workspaces(cmd.Context())
	if err != nil {
		return err
	}

	ws, err := selectWorkspace(out, in, workspaces)
	if err != nil {
		return err
	}

	// Preserve any existing personal settings (push rounding, mapping
	// overrides) and update only the credentials and workspace.
	uc, err := config.LoadUser()
	if err != nil {
		return err
	}
	uc.APIKey = apiKey
	uc.WorkspaceID = ws.ID
	if err := config.SaveUser(uc); err != nil {
		return err
	}

	path, err := config.UserConfigPath()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Authenticated. Using workspace %q (saved to %s).\n", ws.Name, path)
	return nil
}

// promptAPIKey reads the Clockify API key from in, prompting on out.
func promptAPIKey(out io.Writer, in *bufio.Reader) (string, error) {
	fmt.Fprint(out, "Clockify API key: ")
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// selectWorkspace resolves which workspace to pin. With none it errors, with one
// it auto-selects, and with several it prints a numbered list and reads a choice
// from in. It is decoupled from the network so it is directly testable.
func selectWorkspace(out io.Writer, in *bufio.Reader, workspaces []clockify.Workspace) (clockify.Workspace, error) {
	switch len(workspaces) {
	case 0:
		return clockify.Workspace{}, fmt.Errorf("no workspaces found for this API key")
	case 1:
		return workspaces[0], nil
	}

	fmt.Fprintln(out, "Select a workspace:")
	for i, ws := range workspaces {
		fmt.Fprintf(out, "  %d) %s\n", i+1, ws.Name)
	}
	fmt.Fprintf(out, "Workspace [1-%d]: ", len(workspaces))

	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return clockify.Workspace{}, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(workspaces) {
		return clockify.Workspace{}, fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
	return workspaces[choice-1], nil
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	rootCmd.AddCommand(authCmd)
}
