package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/biotools/brun/internal"
	"github.com/spf13/cobra"
)

// Template constants defined in helptext.go (helpTemplate, usageTemplate)

type Options struct {
	Version string
}

func Execute(opts Options) error {
	if err := internal.InitLogger(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化日志: %v\n", err)
	}

	rootCmd := &cobra.Command{
		Version:       opts.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			if c.ArgsLenAtDash() < 0 {
				return cliError("missing_command_separator", "未知命令或缺少 -- 分隔符", "运行命令请使用 brun -- <command>；查看帮助使用 brun --help", nil)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return c.Help()
			}
			return detachRun(c, args, "", "", "", nil, false, "", 0, "")
		},
	}
	MustParse("root").Inject(rootCmd)
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetUsageTemplate(usageTemplate)

	rootCmd.AddCommand(
		initCmd(),
		runCmd(),
		stopCmd(),
		listCmd(),
		showCmd(),
		scriptCmd(),
		logsCmd(),
		outputsCmd(),
		diagCmd(),
		tagCmd(),
		noteCmd(),
		rerunCmd(),
		cleanCmd(),
		repairCmd(),
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

	if err := rootCmd.Execute(); err != nil {
		if isRootShortcutFlagError(rootCmd, err, os.Args[1:]) {
			err = cliError("missing_command_separator", "未知命令或缺少 -- 分隔符", "运行命令请使用 brun -- <command>；查看帮助使用 brun --help", err)
		}
		fmt.Fprint(os.Stderr, formatCLIError(err))
		return err
	}
	return nil
}

func isRootShortcutFlagError(rootCmd *cobra.Command, err error, args []string) bool {
	if err == nil || len(args) == 0 {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown shorthand flag") && !strings.Contains(msg, "unknown flag") {
		return false
	}
	first := args[0]
	if strings.HasPrefix(first, "-") {
		return false
	}
	for _, arg := range args {
		if arg == "--" {
			return false
		}
	}
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == first || sub.HasAlias(first) {
			return false
		}
	}
	return true
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
