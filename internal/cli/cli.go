// Package cli defines the gh-tree commands and wires fetching, building, and
// rendering together (docs/prd.md 5章).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"

	"github.com/ohzw/gh-tree/internal/build"
	"github.com/ohzw/gh-tree/internal/github"
	"github.com/ohzw/gh-tree/internal/model"
	"github.com/ohzw/gh-tree/internal/render"
)

type options struct {
	json        bool
	noColor     bool
	depth       int
	labels      []string
	hideClosed  bool
	noAncestors bool
	state       string
	issues      bool
}

// Execute runs the command line and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := newRootCmd()
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "gh-tree:", err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	o := &options{}
	root := &cobra.Command{
		Use:         "tree [<number>]",
		Annotations: map[string]string{cobra.CommandDisplayNameAnnotation: "gh tree"},
		Short:       "Show issue, pull request, and branch relationships as a tree",
		Long: `Show GitHub issues (sub-issues and linked pull requests), pull requests
(stacked by base and head branch), and branches as a colored tree.

With a bare <number>, gh tree detects whether it is an issue or a pull request.`,
		Example: `  gh tree issue 1244
  gh tree pr
  gh tree pr 1257
  gh tree branch develop --issues
  gh tree 1244 --json`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return fmt.Errorf("unknown command %q; run 'gh tree --help' for usage", args[0])
			}
			return runAuto(cmd, o, n)
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&o.json, "json", false, "Output JSON")
	pf.BoolVar(&o.noColor, "no-color", false, "Disable color (also honors NO_COLOR)")
	pf.IntVarP(&o.depth, "depth", "d", -1, "Depth to traverse, counting the root as 0; -1 is unlimited")
	pf.StringSliceVar(&o.labels, "labels", nil, "Comma-separated labels to show next to nodes (does not filter)")

	// A bare <number> may resolve to either subcommand, so it accepts both sets of flags.
	addIssueFlags(root, o)
	addPRFlags(root, o)

	root.AddCommand(newIssueCmd(o), newPRCmd(o), newBranchCmd(o))
	return root
}

func addIssueFlags(cmd *cobra.Command, o *options) {
	cmd.Flags().BoolVar(&o.hideClosed, "hide-closed", false, "Fold closed sub-issues into one line")
	cmd.Flags().BoolVar(&o.noAncestors, "no-ancestors", false, "Do not show the parent issues above the requested issue")
}

func addPRFlags(cmd *cobra.Command, o *options) {
	cmd.Flags().StringVar(&o.state, "state", "open", "Pull requests to show: open|merged|all")
	cmd.Flags().BoolVar(&o.issues, "issues", false, "Show issues closed by each pull request as its children")
}

func newIssueCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <number>...",
		Short: "Show the sub-issue tree and linked pull requests of issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nums, err := parseNumbers(args)
			if err != nil {
				return err
			}
			return runIssues(cmd, o, nums)
		},
	}
	addIssueFlags(cmd, o)
	return cmd
}

func newPRCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr [<number>...]",
		Short: "Show pull requests stacked by base and head branch",
		Long: `Show pull requests stacked by base and head branch.

Without numbers, every pull request in --state is shown. With numbers, only the
path from the root branch to each pull request and the tree below it is shown.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			nums, err := parseNumbers(args)
			if err != nil {
				return err
			}
			return runPRs(cmd, o, nums)
		},
	}
	addPRFlags(cmd, o)
	return cmd
}

func newBranchCmd(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch <name>",
		Short: "Show the pull request tree rooted at a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranch(cmd, o, args[0])
		},
	}
	addPRFlags(cmd, o)
	return cmd
}

func parseNumbers(args []string) ([]int, error) {
	nums := make([]int, len(args))
	for i, a := range args {
		n, err := strconv.Atoi(a)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid number %q", a)
		}
		nums[i] = n
	}
	return nums, nil
}

// session is the resolved repository and API client of one invocation.
type session struct {
	repo   string
	client *github.Client
	// defaultBranch is set by pullRequests for text output of PR trees.
	defaultBranch string
}

func newSession() (*session, error) {
	r, err := repository.Current()
	if err != nil {
		return nil, fmt.Errorf("could not determine the repository (run inside a GitHub repository or set GH_REPO=owner/name): %w", err)
	}
	repo := r.Owner + "/" + r.Name
	c, err := github.NewClient(repo)
	if err != nil {
		return nil, err
	}
	return &session{repo: repo, client: c}, nil
}

func (o *options) buildOptions() build.Options {
	return build.Options{
		Depth:      o.depth,
		HideClosed: o.hideClosed,
		Ancestors:  !o.noAncestors,
		WithIssues: o.issues,
		Labels:     o.labels,
	}
}

func (o *options) prStates() ([]string, error) {
	switch o.state {
	case "open":
		return []string{"OPEN"}, nil
	case "merged":
		return []string{"MERGED"}, nil
	case "all":
		return []string{"OPEN", "MERGED", "CLOSED"}, nil
	default:
		return nil, fmt.Errorf("invalid --state %q: want open, merged, or all", o.state)
	}
}

func runAuto(cmd *cobra.Command, o *options, n int) error {
	s, err := newSession()
	if err != nil {
		return err
	}
	kind, err := s.client.Kind(cmd.Context(), n)
	if err != nil {
		return err
	}
	switch kind {
	case model.KindIssue:
		return s.issues(cmd, o, []int{n})
	case model.KindPR:
		return s.prs(cmd, o, []int{n})
	default:
		return fmt.Errorf("#%d is neither an issue nor a pull request", n)
	}
}

func runIssues(cmd *cobra.Command, o *options, nums []int) error {
	s, err := newSession()
	if err != nil {
		return err
	}
	return s.issues(cmd, o, nums)
}

func runPRs(cmd *cobra.Command, o *options, nums []int) error {
	s, err := newSession()
	if err != nil {
		return err
	}
	return s.prs(cmd, o, nums)
}

func runBranch(cmd *cobra.Command, o *options, name string) error {
	states, err := o.prStates()
	if err != nil {
		return err
	}
	s, err := newSession()
	if err != nil {
		return err
	}
	prs, err := s.pullRequests(cmd.Context(), o, states)
	if err != nil {
		return err
	}
	root, err := build.BranchTree(prs, name, o.buildOptions())
	if err != nil {
		return err
	}
	return s.write(cmd.OutOrStdout(), o, []*model.Node{root})
}

// issues fetches every root issue concurrently and prints them in argument order.
func (s *session) issues(cmd *cobra.Command, o *options, nums []int) error {
	trees := make([]*model.Issue, len(nums))
	errs := make([]error, len(nums))
	var wg sync.WaitGroup
	for i, n := range nums {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trees[i], errs[i] = s.client.IssueTree(cmd.Context(), n, o.depth)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}

	bo := o.buildOptions()
	roots := make([]*model.Node, len(trees))
	for i, t := range trees {
		roots[i] = build.IssueTree(t, bo)
	}
	return s.write(cmd.OutOrStdout(), o, roots)
}

// pullRequests fetches the PRs in states. For text output it also fetches the
// default branch concurrently so its branch root is rendered unannotated.
func (s *session) pullRequests(ctx context.Context, o *options, states []string) ([]model.PR, error) {
	if o.json {
		return s.client.PullRequests(ctx, states)
	}
	var wg sync.WaitGroup
	var branchErr error
	wg.Go(func() {
		s.defaultBranch, branchErr = s.client.DefaultBranch(ctx)
	})
	prs, err := s.client.PullRequests(ctx, states)
	wg.Wait()
	if err := errors.Join(err, branchErr); err != nil {
		return nil, err
	}
	return prs, nil
}

// prs prints the whole PR forest, or with nums, the focused tree of each PR.
// A requested PR outside --state is fetched on its own so it can still be placed.
func (s *session) prs(cmd *cobra.Command, o *options, nums []int) error {
	states, err := o.prStates()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	prs, err := s.pullRequests(ctx, o, states)
	if err != nil {
		return err
	}
	bo := o.buildOptions()
	if len(nums) == 0 {
		return s.write(cmd.OutOrStdout(), o, build.PRForest(prs, bo))
	}

	have := make(map[int]bool, len(prs))
	for _, p := range prs {
		have[p.Number] = true
	}
	for _, n := range nums {
		if have[n] {
			continue
		}
		p, err := s.client.PullRequest(ctx, n)
		if err != nil {
			return err
		}
		prs = append(prs, p)
		have[n] = true
	}

	roots := make([]*model.Node, len(nums))
	for i, n := range nums {
		if roots[i], err = build.PRFocus(prs, n, bo); err != nil {
			return err
		}
	}
	return s.write(cmd.OutOrStdout(), o, roots)
}

func (s *session) write(w io.Writer, o *options, roots []*model.Node) error {
	if o.json {
		return render.JSON(w, s.repo, roots)
	}
	t := term.FromEnv()
	color := !o.noColor && t.IsColorEnabled()
	width := 0
	if t.IsTerminalOutput() {
		if cols, _, err := t.Size(); err == nil {
			width = cols
		}
	}
	return render.Text(w, roots, render.Options{Color: color, Repo: s.repo, DefaultBranch: s.defaultBranch, Width: width})
}
