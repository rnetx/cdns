package cdns

import (
	"fmt"
	"strings"

	"github.com/rnetx/cdns/constant"
	"github.com/rnetx/cdns/plugin"

	"github.com/spf13/cobra"
)

var versionCommand = &cobra.Command{
	Use: "version",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(fmt.Sprintf("cdns %s", constant.Version))
		fmt.Println(fmt.Sprintf("plugin matcher: %s", strings.Join(plugin.PluginMatcherTypes(), ", ")))
		fmt.Println(fmt.Sprintf("plugin executor: %s", strings.Join(plugin.PluginExecutorTypes(), ", ")))
	},
}
