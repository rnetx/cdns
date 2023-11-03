package cdns

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/rnetx/cdns/constant"
	"github.com/rnetx/cdns/core"
	"github.com/rnetx/cdns/log"
	"github.com/rnetx/cdns/plugin"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var MainCommand = &cobra.Command{
	Use: "cdns",
	Run: func(_ *cobra.Command, _ []string) {
		code := run()
		if code != 0 {
			os.Exit(code)
		}
	},
}

var configPath string

func init() {
	//
	{
		e, err := strconv.ParseBool(os.Getenv("CDNS_LISTENER_ENABLE_PANIC"))
		if err == nil && e {
			constant.ListenerEnablePainc = true
		}
	}
	//
	MainCommand.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "config file path")
	MainCommand.AddCommand(versionCommand)
}

func run() int {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		log.DefaultLogger.Errorf("read config file failed: %s, error: %s", configPath, err)
		return 1
	}
	var options core.Options
	err = yaml.Unmarshal(raw, &options)
	if err != nil {
		log.DefaultLogger.Errorf("parse config file failed: %s, error: %s", configPath, err)
		return 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, coreLogger, err := core.NewCore(ctx, options)
	if err != nil {
		log.DefaultLogger.Error(err)
		return 1
	}
	coreLogger.Infof("cdns %s", constant.Version)
	coreLogger.Infof("plugin matcher: %s", strings.Join(plugin.PluginMatcherTypes(), ", "))
	coreLogger.Infof("plugin executor: %s", strings.Join(plugin.PluginExecutorTypes(), ", "))
	if constant.ListenerEnablePainc {
		coreLogger.Infof("debug: listener enable painc")
	}
	go signalHandle(cancel, coreLogger)
	err = c.Run()
	if err != nil {
		return 1
	}
	return 0
}

func signalHandle(cancel context.CancelFunc, logger log.Logger) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, os.Interrupt)
	<-signalChan
	logger.Warn("receive signal, exiting...")
	cancel()
}
