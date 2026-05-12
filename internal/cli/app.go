package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Mohabdo21/gh-search/internal/githubsearch"
	"github.com/Mohabdo21/gh-search/internal/render"
	"golang.org/x/term"
)

const (
	stateAll    = "all"
	stateOpen   = "open"
	stateClosed = "closed"

	typeAll        = "all"
	typeIssue      = "issue"
	typeDiscussion = "discussion"
)

type config struct {
	Query      string
	State      string
	Type       string
	RepoURL    string
	Sort       string
	Limit      int
	JSON       bool
	PlainLinks bool
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if stdout == nil {
		return errors.New("stdout writer is required")
	}
	if stderr == nil {
		return errors.New("stderr writer is required")
	}

	conf, err := parseArgs(args, stderr)
	if err != nil {
		return err
	}

	repo, err := githubsearch.ParseRepositoryURL(conf.RepoURL)
	if err != nil {
		return err
	}

	client, err := githubsearch.NewClient(repo.Host)
	if err != nil {
		return fmt.Errorf("create GitHub client: %w", err)
	}

	searchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	result := render.Result{
		Query: conf.Query,
		Repo:  repo.FullName,
		Sort:  conf.Sort,
		Limit: conf.Limit,
	}

	if conf.Type == typeAll || conf.Type == typeIssue {
		issues, err := client.SearchIssues(searchCtx, repo, githubsearch.SearchOptions{
			Keywords: conf.Query,
			State:    conf.State,
			Sort:     conf.Sort,
			Limit:    conf.Limit,
		})
		if err != nil {
			return err
		}
		result.Issues = issues
	}

	if conf.Type == typeAll || conf.Type == typeDiscussion {
		discussions, err := client.SearchDiscussions(searchCtx, repo, githubsearch.SearchOptions{
			Keywords: conf.Query,
			State:    conf.State,
			Sort:     conf.Sort,
			Limit:    conf.Limit,
		})
		if err != nil {
			return err
		}
		result.Discussions = discussions
	}

	return render.Write(stdout, result, render.Options{
		JSON:       conf.JSON,
		Hyperlinks: !conf.JSON && !conf.PlainLinks && supportsHyperlinks(stdout),
	})
}

func supportsHyperlinks(w io.Writer) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	ttyWriter, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}

	return term.IsTerminal(int(ttyWriter.Fd()))
}

func parseArgs(args []string, stderr io.Writer) (config, error) {
	conf := config{}

	fs := flag.NewFlagSet("gh-search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&conf.Query, "query", "", "Keywords to search for")
	fs.StringVar(&conf.Query, "q", "", "Short form of --query")
	fs.StringVar(&conf.State, "state", stateAll, "Filter results by state: open, closed, or all")
	fs.StringVar(&conf.State, "s", stateAll, "Short form of --state")
	fs.StringVar(&conf.Type, "type", typeAll, "Search issues, discussions, or both: issue, discussion, or all")
	fs.StringVar(&conf.Type, "t", typeAll, "Short form of --type")
	fs.StringVar(&conf.Sort, "sort", "relevance", "Sort results by relevance, created[-asc|-desc], or updated[-asc|-desc]")
	fs.IntVar(&conf.Limit, "limit", 0, "Maximum results to return per selected result type; 0 means no explicit limit")
	fs.BoolVar(&conf.JSON, "json", false, "Emit JSON instead of table output")
	fs.BoolVar(&conf.PlainLinks, "plain-links", false, "Disable terminal hyperlinks and always print the plain Direct Links section")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage:")
		_, _ = fmt.Fprintln(stderr, "  gh-search <repo-url> (--query|-q) <keywords> [(--state|-s) open|closed|all] [(--type|-t) issue|discussion|all] [--limit N] [--sort relevance|created|created-asc|created-desc|updated|updated-asc|updated-desc] [--json] [--plain-links]")
		_, _ = fmt.Fprintln(stderr)
		_, _ = fmt.Fprintln(stderr, "Examples:")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/moby/moby --query \"container exit code\" --state all")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/kubernetes/kubernetes -q \"pod pending\" -t discussion")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/kubernetes/kubernetes --query \"pod pending\" --type discussion")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/cli/cli --query \"extension\" --limit 5 --sort created-asc")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/cli/go-gh --query \"graphql client\" --json --type issue")
		_, _ = fmt.Fprintln(stderr, "  gh-search https://github.com/hyprwm/Hyprland --query \"GPU Reset\" --limit 4 --plain-links")
	}

	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		conf.RepoURL = strings.TrimSpace(args[0])
		parseArgs = args[1:]
	}

	if err := fs.Parse(parseArgs); err != nil {
		return conf, err
	}

	conf.State = strings.ToLower(strings.TrimSpace(conf.State))
	conf.Type = strings.ToLower(strings.TrimSpace(conf.Type))
	conf.Query = strings.TrimSpace(conf.Query)
	conf.Sort = strings.ToLower(strings.TrimSpace(conf.Sort))

	positionals := fs.Args()
	if conf.RepoURL == "" {
		if len(positionals) != 1 {
			fs.Usage()
			return conf, errors.New("exactly one repository URL is required")
		}
		conf.RepoURL = strings.TrimSpace(positionals[0])
	} else if len(positionals) != 0 {
		fs.Usage()
		return conf, errors.New("exactly one repository URL is required")
	}
	if conf.Query == "" {
		fs.Usage()
		return conf, errors.New("--query is required")
	}

	if !isOneOf(conf.State, stateAll, stateOpen, stateClosed) {
		return conf, fmt.Errorf("invalid --state value %q: expected open, closed, or all", conf.State)
	}
	if !isOneOf(conf.Type, typeAll, typeIssue, typeDiscussion) {
		return conf, fmt.Errorf("invalid --type value %q: expected issue, discussion, or all", conf.Type)
	}
	canonicalSort, err := normalizeSort(conf.Sort)
	if err != nil {
		return conf, err
	}
	conf.Sort = canonicalSort
	if conf.Limit < 0 {
		return conf, fmt.Errorf("invalid --limit value %s: expected 0 or a positive integer", strconv.Itoa(conf.Limit))
	}

	return conf, nil
}

func normalizeSort(value string) (string, error) {
	switch value {
	case "", "relevance", "relevance-desc":
		return "relevance", nil
	case "created":
		return "created-desc", nil
	case "created-asc", "created-desc":
		return value, nil
	case "updated":
		return "updated-desc", nil
	case "updated-asc", "updated-desc":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --sort value %q: expected relevance, created[-asc|-desc], or updated[-asc|-desc]", value)
	}
}

func isOneOf(value string, valid ...string) bool {
	return slices.Contains(valid, value)
}
