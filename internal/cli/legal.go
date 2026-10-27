package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/eosaios/eos/internal/version"
	"github.com/spf13/cobra"
)

// hiddenLegalWatermark 是内嵌在二进制中的隐蔽版权水印。
// 即使可见品牌被篡改，通过 strings 命令仍可追溯来源。
var hiddenLegalWatermark = "EOS-COPYRIGHT:Copyright(c)2026 EOSAIOS License:EOS-NCL-1.1 Contact:legal@eosaios.com"

func newHiddenLegalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "_legal",
		Short:  "",
		Long:   "",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Copyright (c) 2026 EOSAIOS")
			fmt.Println("License: EOS Non-Commercial License v1.1 (EOS-NCL-1.1)")
			fmt.Println("SPDX-License-Identifier: EOS-NCL-1.1")
			fmt.Println("Contact: legal@eosaios.com")
			fmt.Println()
			fmt.Printf("Version:     %s\n", version.AppVersion)
			fmt.Printf("BuildCommit: %s\n", version.BuildCommit)
			fmt.Printf("BuildDate:   %s\n", version.BuildDate)
			fmt.Printf("GoVersion:   %s\n", goVersion())
			fmt.Println()

			// 引用水印变量确保它不被编译器优化掉
			fmt.Printf("WATERMARK:   %s\n", hiddenLegalWatermark)
		},
	}
	return cmd
}

func goVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.GoVersion
	}
	return "unknown"
}
