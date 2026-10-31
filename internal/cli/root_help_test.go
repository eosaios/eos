package cli

import (
	"strings"
	"testing"
)

func TestRootHelpLocalizedFlags(t *testing.T) {
	t.Setenv("EOS_LANG", "zh")
	configureRootHelp(rootCmd)
	for _, cmd := range rootCmd.Commands() {
		if !cmd.Hidden && strings.HasPrefix(cmd.Short, "cli.command.") {
			t.Fatalf("missing command translation: %s", cmd.Name())
		}
	}
	for _, name := range []string{"print", "version", "approval-mode", "help"} {
		flag := rootCmd.Flags().Lookup(name)
		if flag == nil {
			flag = rootCmd.PersistentFlags().Lookup(name)
		}
		if flag == nil || strings.HasPrefix(flag.Usage, "cli.flag.") || !strings.ContainsAny(flag.Usage, "执行显示审批") {
			t.Fatalf("unlocalized %s: %v", name, flag)
		}
	}
	t.Setenv("EOS_LANG", "en")
	configureRootHelp(rootCmd)
	if rootCmd.PersistentFlags().Lookup("print").Usage != "Run a single query in headless mode and print the result" {
		t.Fatal("English help lost")
	}
}
