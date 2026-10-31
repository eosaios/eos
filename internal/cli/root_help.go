package cli

import (
	"github.com/eosaios/eos/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"strings"
)

func configureRootHelp(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultCompletionCmd()
	cmd.Short = rootShort()
	cmd.Long = rootLong()
	for _, command := range cmd.Commands() {
		if !command.Hidden {
			command.Short = ui.T("cli.command."+command.Name(), rootLang())
		}
	}
	for _, flags := range []*pflag.FlagSet{cmd.PersistentFlags(), cmd.LocalNonPersistentFlags()} {
		flags.VisitAll(func(flag *pflag.Flag) { flag.Usage = ui.T("cli.flag."+flag.Name, rootLang()) })
	}
	if rootLang() == "zh" {
		cmd.SetUsageTemplate(strings.NewReplacer("Usage:", "用法：", "Available Commands:", "可用命令：", "Global Flags:", "全局选项：", "Flags:", "选项：", "Use ", "使用 ", " for more information about a command.", " 查看命令详情。").Replace(cmd.UsageTemplate()))
	}
}
