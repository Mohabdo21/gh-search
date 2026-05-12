package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Mohabdo21/gh-search/internal/githubsearch"
)

const titleColumnWidth = 60

type Result struct {
	Query       string                          `json:"query"`
	Repo        string                          `json:"repo"`
	Sort        string                          `json:"sort,omitempty"`
	Limit       int                             `json:"limit,omitempty"`
	Issues      []githubsearch.IssueResult      `json:"issues,omitempty"`
	Discussions []githubsearch.DiscussionResult `json:"discussions,omitempty"`
}

type Options struct {
	JSON            bool
	Hyperlinks      bool
	ShowIssues      bool
	ShowDiscussions bool
}

func Write(w io.Writer, result Result, opts Options) error {
	if opts.JSON {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}

	if _, err := fmt.Fprintf(w, "Search Results for %q in %s\n\n", result.Query, result.Repo); err != nil {
		return err
	}

	showIssues, showDiscussions := visibleSections(opts)
	sectionsWritten := 0

	if showIssues {
		if err := writeIssueSection(w, result.Issues, opts.Hyperlinks); err != nil {
			return err
		}
		sectionsWritten++
	}
	if showDiscussions {
		if sectionsWritten > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if err := writeDiscussionSection(w, result.Discussions, opts.Hyperlinks); err != nil {
			return err
		}
		sectionsWritten++
	}
	if opts.Hyperlinks {
		return nil
	}
	if sectionsWritten > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return writeLinks(w, result)
}

func visibleSections(opts Options) (bool, bool) {
	if !opts.ShowIssues && !opts.ShowDiscussions {
		return true, true
	}

	return opts.ShowIssues, opts.ShowDiscussions
}

func writeIssueSection(w io.Writer, issues []githubsearch.IssueResult, hyperlinks bool) error {
	if _, err := fmt.Fprintf(w, "ISSUES (%d results)\n", len(issues)); err != nil {
		return err
	}
	if len(issues) == 0 {
		_, err := fmt.Fprintln(w, "No matching issues found.")
		return err
	}

	rows := make([][]string, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, []string{
			fmt.Sprintf("#%d", issue.Number),
			linkifyTitle(truncate(issue.Title, titleColumnWidth), issue.URL, hyperlinks),
			issue.State,
			issue.CreatedAt.Format("2006-01-02"),
		})
	}

	return writeTable(w, []string{"Number", "Title", "State", "Created"}, rows)
}

func writeDiscussionSection(w io.Writer, discussions []githubsearch.DiscussionResult, hyperlinks bool) error {
	if _, err := fmt.Fprintf(w, "DISCUSSIONS (%d results)\n", len(discussions)); err != nil {
		return err
	}
	if len(discussions) == 0 {
		_, err := fmt.Fprintln(w, "No matching discussions found.")
		return err
	}

	rows := make([][]string, 0, len(discussions))
	for _, discussion := range discussions {
		rows = append(rows, []string{
			fmt.Sprintf("#%d", discussion.Number),
			linkifyTitle(truncate(discussion.Title, titleColumnWidth), discussion.URL, hyperlinks),
			discussion.State,
			discussion.CreatedAt.Format("2006-01-02"),
		})
	}

	return writeTable(w, []string{"Number", "Title", "State", "Created"}, rows)
}

func writeLinks(w io.Writer, result Result) error {
	if _, err := fmt.Fprintln(w, "Direct Links:"); err != nil {
		return err
	}
	if len(result.Issues) == 0 && len(result.Discussions) == 0 {
		_, err := fmt.Fprintln(w, "  none")
		return err
	}

	for _, issue := range result.Issues {
		if _, err := fmt.Fprintf(w, "  %s\n", issue.URL); err != nil {
			return err
		}
	}
	for _, discussion := range result.Discussions {
		if _, err := fmt.Fprintf(w, "  %s\n", discussion.URL); err != nil {
			return err
		}
	}

	return nil
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = displayWidth(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if width := displayWidth(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}

	if err := printBorder(w, widths); err != nil {
		return err
	}
	if err := printRow(w, widths, headers); err != nil {
		return err
	}
	if err := printBorder(w, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := printRow(w, widths, row); err != nil {
			return err
		}
	}
	return printBorder(w, widths)
}

func printBorder(w io.Writer, widths []int) error {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("-", width+2))
	}
	return fprintf(w, "+%s+\n", strings.Join(parts, "+"))
}

func printRow(w io.Writer, widths []int, row []string) error {
	columns := make([]string, 0, len(row))
	for i, cell := range row {
		padding := widths[i] - displayWidth(cell)
		columns = append(columns, " "+cell+strings.Repeat(" ", padding+1))
	}
	return fprintf(w, "|%s|\n", strings.Join(columns, "|"))
}

func truncate(value string, limit int) string {
	if limit <= 0 || displayWidth(value) <= limit {
		return value
	}

	if limit <= 3 {
		return value[:limit]
	}

	count := 0
	for i := range value {
		if count == limit-3 {
			return value[:i] + "..."
		}
		count++
	}

	return value
}

func linkifyTitle(title, url string, enabled bool) string {
	if !enabled || title == "" || url == "" {
		return title
	}

	return "\x1b]8;;" + url + "\x1b\\" + title + "\x1b]8;;\x1b\\"
}

func displayWidth(value string) int {
	return utf8.RuneCountInString(stripTerminalSequences(value))
}

func fprintf(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func stripTerminalSequences(value string) string {
	var builder strings.Builder

	for i := 0; i < len(value); {
		if value[i] != '\x1b' {
			builder.WriteByte(value[i])
			i++
			continue
		}

		if i+1 >= len(value) {
			break
		}

		switch value[i+1] {
		case ']':
			i += 2
			for i < len(value) {
				if value[i] == '\a' {
					i++
					break
				}
				if value[i] == '\x1b' && i+1 < len(value) && value[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case '[':
			i += 2
			for i < len(value) {
				if value[i] >= 0x40 && value[i] <= 0x7e {
					i++
					break
				}
				i++
			}
		default:
			i += 2
		}
	}

	return builder.String()
}
