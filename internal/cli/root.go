package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/biotools/brun/internal"
	"github.com/spf13/cobra"
)

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if .Example}}
示例:
{{.Example}}
{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

const usageTemplate = `用法: {{.UseLine}}

{{if .HasAvailableSubCommands}}
可用命令:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableLocalFlags}}
选项:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasAvailableInheritedFlags}}
全局选项:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasHelpSubCommands}}
更多帮助命令:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableSubCommands}}
使用 "{{.CommandPath}} [命令] --help" 获取更多信息
{{end}}
`

type Options struct {
	Version string
}

func Execute(opts Options) error {
	if err := internal.InitLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化日志: %v\n", err)
	}

	rootCmd := &cobra.Command{
		Use:   "brun",
		Short: "bio-runner: 面向生物信息学的运行记录与日志管理工具",
		Long: `brun 是一个跨项目运行记录工具。
	通过 brun run -- <command> 包装任意命令，自动记录日志、环境、Git 信息和输出文件。`,
		Version: opts.Version,
	}
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetUsageTemplate(usageTemplate)

	rootCmd.AddCommand(
		initCmd(),
		runCmd(),
		listCmd(),
		showCmd(),
		scriptCmd(),
		logsCmd(),
		outputsCmd(),
		tagCmd(),
		noteCmd(),
		rerunCmd(),
		cleanCmd(),
		webCmd(),
	)
	// 替换内置命令为中文描述
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "查看帮助信息",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return rootCmd.Help()
			}
			c2, _, err := rootCmd.Find(args)
			if err != nil {
				return err
			}
			return c2.Help()
		},
	})
	// 禁用 cobra 默认的英文 help/version flag，改用中文版本
	rootCmd.PersistentFlags().BoolP("help", "h", false, "显示帮助信息")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "生成指定 shell 的自动补全脚本",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("不支持的 shell: %s (支持: bash, zsh, fish, powershell)", args[0])
			}
		},
	})
	rootCmd.RegisterFlagCompletionFunc("help", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.PersistentFlags().BoolP("version", "v", false, "显示版本号")

	return rootCmd.Execute()
}

func openStore() (*internal.Store, error) {
	return internal.NewStore(filepath.Join(internal.HomeDir(), "db.sqlite"))
}

func openStoreReadOnly() (*internal.Store, error) {
	path := filepath.Join(internal.HomeDir(), "db.sqlite")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return internal.NewStore(path)
		}
		return nil, err
	}
	return internal.OpenStoreReadOnly(path)
}

// --- init ---
