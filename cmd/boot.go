package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	_ "time/tzdata"

	"glance/internal/desktoplaunch"

	"github.com/spf13/cobra"
	"github.com/wonli/aqi/service"
)

var timezone string
var configFile string

func init() {
	rootCmd.PersistentFlags().StringVarP(&timezone, "tz", "t", "default", "指定时区")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.yaml", "指定配置文件路径，默认当前路径下config.yaml")
	rootCmd.AddCommand(service.Command(service.CommandOptions{
		Self: true,
		Config: func() string {
			return configFile
		},
	}))
	cobra.OnInitialize(func() {
	})
}

var runApp = filepath.Base(strings.TrimLeft(os.Args[0], "./"))
var rootCmd = &cobra.Command{
	Use:     runApp,
	Version: "glance",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) == 1 {
			path = args[0]
		}
		desktoplaunch.RunDesktop(path)
		return nil
	},
}

func Execute() {
	if len(os.Args) == 1 {
		// A double-clicked GUI executable has no CLI arguments. Launch the
		// desktop directly instead of relying on Cobra's no-argument dispatch.
		desktoplaunch.RunDesktop("")
		return
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
