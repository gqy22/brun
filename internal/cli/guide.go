package cli

import (
	"fmt"
	"strings"

	"github.com/biotools/brun/internal/guide"
	"github.com/spf13/cobra"
)

func guideCmd() *cobra.Command {
	rootHelp := MustParse("guide")
	c := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return c.Help()
		},
	}
	rootHelp.Inject(c)
	c.AddCommand(guideListCmd(), guideShowCmd(), guideSearchCmd())
	return c
}

func guideListCmd() *cobra.Command {
	var tool, category string
	ht := MustParse("guide-list")
	c := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			entries, err := guide.Load()
			if err != nil {
				return err
			}
			entries = guide.Filter(entries, tool, category)
			if len(entries) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "没有符合条件的内置经验。")
				return nil
			}
			fmt.Fprint(c.OutOrStdout(), formatGuideEntries("内置经验", entries))
			return nil
		},
	}
	c.Flags().StringVar(&tool, "tool", "", "按工具过滤")
	c.Flags().StringVar(&category, "category", "", "按分类过滤")
	ht.Inject(c)
	return c
}

func guideShowCmd() *cobra.Command {
	ht := MustParse("guide-show")
	c := &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			entries, err := guide.Load()
			if err != nil {
				return err
			}
			entry, ok := guide.Find(entries, args[0])
			if !ok {
				return cliError(
					"guide_not_found",
					fmt.Sprintf("找不到内置经验 %q", args[0]),
					"使用 brun guide list 查看全部主题，或 brun guide search <关键词> 搜索",
					nil,
				)
			}
			fmt.Fprint(c.OutOrStdout(), formatGuideEntry(entry))
			return nil
		},
	}
	ht.Inject(c)
	return c
}

func guideSearchCmd() *cobra.Command {
	ht := MustParse("guide-search")
	c := &cobra.Command{
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			entries, err := guide.Load()
			if err != nil {
				return err
			}
			matches := guide.Search(entries, args[0])
			if len(matches) == 0 {
				fmt.Fprintf(c.OutOrStdout(), "没有找到包含 %q 的内置经验。\n", args[0])
				return nil
			}
			fmt.Fprint(c.OutOrStdout(), formatGuideEntries(fmt.Sprintf("搜索结果: %s", args[0]), matches))
			return nil
		},
	}
	ht.Inject(c)
	return c
}

// formatGuideEntries deliberately uses stacked records instead of a wide table,
// so titles and summaries remain readable in terminals narrower than 80 columns.
func formatGuideEntries(title string, entries []guide.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d)\n\n", title, len(entries))
	for i, entry := range entries {
		fmt.Fprintf(&b, "%s\n", entry.ID)
		fmt.Fprintf(&b, "  %s\n", entry.Title)
		fmt.Fprintf(&b, "  %s\n", entry.Summary)
		fmt.Fprintf(&b, "  %s · %s · %s\n", entry.Tool, entry.Category, entry.Status)
		if i != len(entries)-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func formatGuideEntry(entry guide.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", entry.Title)
	fmt.Fprintf(&b, "%s\n\n", strings.Repeat("=", len([]rune(entry.Title))))
	fmt.Fprintf(&b, "%s\n\n", entry.Summary)
	fmt.Fprintf(&b, "ID:       %s\n", entry.ID)
	fmt.Fprintf(&b, "工具:     %s\n", entry.Tool)
	fmt.Fprintf(&b, "分类:     %s\n", entry.Category)
	fmt.Fprintf(&b, "级别:     %s\n", entry.Level)
	fmt.Fprintf(&b, "状态:     %s\n", entry.Status)
	fmt.Fprintf(&b, "实测版本: %s\n", strings.Join(entry.ToolVersions.Tested, ", "))
	fmt.Fprintf(&b, "适用版本: %s\n", entry.ToolVersions.Applicable)
	fmt.Fprintf(&b, "最后验证: %s\n", entry.Updated)
	fmt.Fprintf(&b, "标签:     %s\n\n", strings.Join(entry.Tags, ", "))
	b.WriteString(renderGuideMarkdown(entry.Body))
	b.WriteByte('\n')
	return b.String()
}

func renderGuideMarkdown(markdown string) string {
	var out []string
	inCode := false
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			out = append(out, "  "+line)
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))+":")
			continue
		}
		line = strings.ReplaceAll(line, "`", "")
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
