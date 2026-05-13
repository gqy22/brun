package main

import (
	"fmt"
	"os"

	"github.com/biotools/brun/cmd"
	"github.com/spf13/cobra"
)

var version = "0.1.0-dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "brun",
		Short: "bio-runner: 面向生物信息学的运行记录与日志管理工具",
		Long: `brun 是一个跨项目运行记录工具。
通过 brun run -- <command> 包装任意命令，自动记录日志、环境、Git 信息和输出文件。`,
		Version: version,
	}

	rootCmd.AddCommand(
		initCmd(),
		runCmd(),
		listCmd(),
		showCmd(),
		logsCmd(),
		outputsCmd(),
		tagCmd(),
		noteCmd(),
		rerunCmd(),
		cleanCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "在当前目录生成 brun.yaml",
		RunE: func(c *cobra.Command, args []string) error {
			project := args[0]
			if len(args) == 0 {
				project = ""
			}
			return cmd.WriteInitFile(".", project)
		},
	}
}

func runCmd() *cobra.Command {
	var (
		name, project, note string
		tags                []string
		inputs, outputs     []string
		noFsDiff            bool
		hashOutputs         bool
		copyOutputs         bool
		timeout             int
		cwd                 string
	)

	c := &cobra.Command{
		Use:   "run -- <command...>",
		Short: "执行命令并记录运行信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("需要指定要执行的命令，使用: brun run -- <command>")
			}
			// TODO: 完整的 run 流程编排
			fmt.Printf("Run: %v\n", args)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "run 名称")
	c.Flags().StringVar(&project, "project", "", "项目名")
	c.Flags().StringVar(&note, "note", "", "备注")
	c.Flags().StringArrayVar(&tags, "tag", []string{}, "标签")
	c.Flags().StringArrayVar(&inputs, "input", []string{}, "输入文件")
	c.Flags().StringArrayVar(&outputs, "output", []string{}, "输出文件")
	c.Flags().BoolVar(&noFsDiff, "no-fs-diff", false, "禁用文件系统 diff")
	c.Flags().BoolVar(&hashOutputs, "hash-outputs", false, "计算输出文件 hash")
	c.Flags().BoolVar(&copyOutputs, "copy-outputs", false, "复制输出文件")
	c.Flags().IntVar(&timeout, "timeout", 0, "超时(秒)")
	c.Flags().StringVar(&cwd, "cwd", "", "运行目录")
	return c
}

func listCmd() *cobra.Command {
	var project, status, tag string
	var limit int

	c := &cobra.Command{
		Use:   "list",
		Short: "列出运行历史",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(cmd.FormatRunList(nil))
			return nil
		},
	}
	c.Flags().StringVar(&project, "project", "", "按项目过滤")
	c.Flags().StringVar(&status, "status", "", "按状态过滤 (success/failed/running)")
	c.Flags().StringVar(&tag, "tag", "", "按 tag 过滤")
	c.Flags().IntVar(&limit, "limit", 20, "限制数量")
	return c
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run_id|latest>",
		Short: "显示运行详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(cmd.FormatShow(nil))
			return nil
		},
	}
}

func logsCmd() *cobra.Command {
	var stdoutOnly, stderrOnly bool
	var tail int
	var follow, openEditor bool

	c := &cobra.Command{
		Use:   "logs <run_id|latest>",
		Short: "查看运行日志",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("logs command")
			return nil
		},
	}
	c.Flags().BoolVar(&stdoutOnly, "stdout", false, "只看 stdout")
	c.Flags().BoolVar(&stderrOnly, "stderr", false, "只看 stderr")
	c.Flags().IntVar(&tail, "tail", 0, "最后 N 行")
	c.Flags().BoolVar(&follow, "follow", false, "持续跟踪")
	c.Flags().BoolVar(&openEditor, "open", false, "用编辑器打开")
	return c
}

func outputsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outputs <run_id|latest>",
		Short: "查看输出文件",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(cmd.FormatOutputs(nil, args[0], ""))
			return nil
		},
	}
}

func tagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tag <run_id|latest> TAG...",
		Short: "添加标签",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Printf("tag %s: %v\n", args[0], args[1:])
			return nil
		},
	}
}

func noteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <run_id|latest> \"text\"",
		Short: "添加备注",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Printf("note %s: %s\n", args[0], args[1])
			return nil
		},
	}
}

func rerunCmd() *cobra.Command {
	var newCWD string
	var dryRun, sameTags bool
	var name string

	c := &cobra.Command{
		Use:   "rerun <run_id|latest>",
		Short: "重新运行",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("rerun %s\n", args[0])
			return nil
		},
	}
	c.Flags().StringVar(&newCWD, "cwd", "", "使用新运行目录")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只打印不执行")
	c.Flags().BoolVar(&sameTags, "with-same-tags", false, "继承原 tags")
	c.Flags().StringVar(&name, "name", "", "指定新 run 名称")
	return c
}

func cleanCmd() *cobra.Command {
	var olderThan string
	var compressLogs bool
	var truncateSize string
	var keepFailed bool
	var keepTag string
	var dryRun bool

	c := &cobra.Command{
		Use:   "clean [options]",
		Short: "清理旧运行记录",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(cmd.FormatCleanSummary(nil, dryRun))
			return nil
		},
	}
	c.Flags().StringVar(&olderThan, "older-than", "", "清理早于此时长的 run")
	c.Flags().BoolVar(&compressLogs, "compress-logs", false, "压缩日志")
	c.Flags().StringVar(&truncateSize, "truncate-large-logs", "", "裁剪超大日志")
	c.Flags().BoolVar(&keepFailed, "keep-failed", false, "保留失败 run")
	c.Flags().StringVar(&keepTag, "keep-tag", "", "保留指定 tag 的 run")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只显示将执行的操作")
	return c
}
