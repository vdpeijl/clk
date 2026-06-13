package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vdpeijl/clk/internal/clockify"
	"github.com/vdpeijl/clk/internal/config"
	"github.com/vdpeijl/clk/internal/fuzzy"
	"github.com/vdpeijl/clk/internal/gitctx"
)

var (
	linkTask     string
	linkPersonal bool
)

var linkCmd = &cobra.Command{
	Use:   "link [token] [project]",
	Short: "Map a local project to a Clockify project (and optional task)",
	Long: `Maps a local project token to a Clockify project so pushed sessions land
in the right place. With no project argument it shows a fuzzy-pick list of your
Clockify projects and remembers the choice — the same prompt-once flow used the
first time an unmapped project would be pushed.

The mapping is written to the committed .clk.toml so teammates inherit it on
pull. Pass --personal to store the choice in ~/.clk instead, where it overrides
the shared mapping locally without changing the committed config.

Forms:
  clk link                       pick a project for the current repo
  clk link <project>             map the current repo to <project>
  clk link <token> <project>     map an explicit project token`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLink(cmd, args)
	},
}

func runLink(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	token, projectArg, err := linkArgs(args)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine working directory: %w", err)
	}
	root := gitctx.Root(cwd)
	if root == "" {
		root = cwd
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("no Clockify API key configured — run `clk auth login` first")
	}
	if cfg.WorkspaceID == "" {
		return fmt.Errorf("no Clockify workspace selected — run `clk auth login` first")
	}

	client := clockify.New(cfg.APIKey, cfg.WorkspaceID)
	projects, err := client.Projects(cmd.Context())
	if err != nil {
		return err
	}

	project, err := chooseProject(cmd, projects, projectArg)
	if err != nil {
		return err
	}

	taskID, err := chooseTask(cmd.Context(), client, project.ID)
	if err != nil {
		return err
	}

	mapping := config.Mapping{Project: project.ID, Task: taskID}
	if linkPersonal {
		if err := config.SetUserMapping(token, mapping); err != nil {
			return err
		}
		fmt.Fprintf(out, "Linked %q -> %s (personal override in ~/.clk).\n", token, project.Name)
		return nil
	}
	if err := config.SetProjectMapping(root, token, mapping); err != nil {
		return err
	}
	fmt.Fprintf(out, "Linked %q -> %s (saved to .clk.toml — commit it to share).\n", token, project.Name)
	return nil
}

// linkArgs resolves the (token, projectArg) pair from the positional arguments.
// With two args the token is explicit; with fewer the current repo's token is
// used and a single arg is the project. An empty projectArg means "prompt".
func linkArgs(args []string) (token, projectArg string, err error) {
	switch len(args) {
	case 2:
		return args[0], args[1], nil
	case 1:
		return currentToken(), args[0], nil
	case 0:
		return currentToken(), "", nil
	default:
		return "", "", fmt.Errorf("too many arguments")
	}
}

// currentToken derives the project token for the current working directory the
// same way `clk init` does, so link and capture agree on the key.
func currentToken() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root := gitctx.Root(cwd)
	if root == "" {
		root = cwd
	}
	return gitctx.Detect(root).ProjectToken
}

// chooseProject resolves projectArg against the live project list, or prompts
// with a fuzzy-pick list when projectArg is empty.
func chooseProject(cmd *cobra.Command, projects []clockify.Project, projectArg string) (clockify.Project, error) {
	if projectArg == "" {
		in := bufio.NewReader(cmd.InOrStdin())
		return pickProject(cmd.OutOrStdout(), in, projects)
	}
	return resolveProject(projects, projectArg)
}

// chooseTask resolves the --task flag against the project's tasks. An empty flag
// leaves the mapping task-less.
func chooseTask(ctx context.Context, client *clockify.Client, projectID string) (string, error) {
	if linkTask == "" {
		return "", nil
	}
	tasks, err := client.Tasks(ctx, projectID)
	if err != nil {
		return "", err
	}
	task, err := resolveTask(tasks, linkTask)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

// pickProject prints a fuzzy-filtered, numbered list of projects and reads a
// selection. It is decoupled from the network and the persistence layer so the
// prompt-once interaction is directly testable.
func pickProject(out io.Writer, in *bufio.Reader, projects []clockify.Project) (clockify.Project, error) {
	if len(projects) == 0 {
		return clockify.Project{}, fmt.Errorf("no Clockify projects found in this workspace")
	}

	fmt.Fprint(out, "Filter projects (blank for all): ")
	query, err := in.ReadString('\n')
	if err != nil && query == "" {
		return clockify.Project{}, err
	}

	ranked := fuzzy.Rank(strings.TrimSpace(query), projectNames(projects))
	if len(ranked) == 0 {
		return clockify.Project{}, fmt.Errorf("no projects match %q", strings.TrimSpace(query))
	}

	fmt.Fprintln(out, "Select a Clockify project:")
	for i, m := range ranked {
		fmt.Fprintf(out, "  %d) %s\n", i+1, projects[m.Index].Name)
	}
	fmt.Fprintf(out, "Project [1-%d]: ", len(ranked))

	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return clockify.Project{}, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(ranked) {
		return clockify.Project{}, fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
	return projects[ranked[choice-1].Index], nil
}

// resolveProject matches arg against the project list: an exact id, then an
// exact (case-insensitive) name, then an unambiguous fuzzy match. It is pure.
func resolveProject(projects []clockify.Project, arg string) (clockify.Project, error) {
	for _, p := range projects {
		if p.ID == arg {
			return p, nil
		}
	}
	var nameMatches []clockify.Project
	for _, p := range projects {
		if strings.EqualFold(p.Name, arg) {
			nameMatches = append(nameMatches, p)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0], nil
	}
	if len(nameMatches) > 1 {
		return clockify.Project{}, fmt.Errorf("%q is ambiguous — pass the project id", arg)
	}

	ranked := fuzzy.Rank(arg, projectNames(projects))
	switch {
	case len(ranked) == 0:
		return clockify.Project{}, fmt.Errorf("no Clockify project matches %q", arg)
	case len(ranked) == 1 || ranked[0].Score > ranked[1].Score:
		return projects[ranked[0].Index], nil
	default:
		return clockify.Project{}, fmt.Errorf("%q matches multiple projects — be more specific or pass the project id", arg)
	}
}

// resolveTask matches arg against a project's tasks with the same precedence as
// resolveProject. It is pure.
func resolveTask(tasks []clockify.Task, arg string) (clockify.Task, error) {
	for _, t := range tasks {
		if t.ID == arg {
			return t, nil
		}
	}
	var nameMatches []clockify.Task
	for _, t := range tasks {
		if strings.EqualFold(t.Name, arg) {
			nameMatches = append(nameMatches, t)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0], nil
	}
	if len(nameMatches) > 1 {
		return clockify.Task{}, fmt.Errorf("task %q is ambiguous — pass the task id", arg)
	}

	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	ranked := fuzzy.Rank(arg, names)
	switch {
	case len(ranked) == 0:
		return clockify.Task{}, fmt.Errorf("no task matches %q", arg)
	case len(ranked) == 1 || ranked[0].Score > ranked[1].Score:
		return tasks[ranked[0].Index], nil
	default:
		return clockify.Task{}, fmt.Errorf("task %q matches multiple tasks — be more specific or pass the task id", arg)
	}
}

// projectNames extracts the names of projects, index-aligned with the input.
func projectNames(projects []clockify.Project) []string {
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names
}

func init() {
	linkCmd.Flags().StringVar(&linkTask, "task", "", "Clockify task name or id to pin alongside the project")
	linkCmd.Flags().BoolVar(&linkPersonal, "personal", false, "store the mapping as a personal override in ~/.clk instead of the committed .clk.toml")
	rootCmd.AddCommand(linkCmd)
}
