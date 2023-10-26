package executor

import (
	_ "github.com/rnetx/cdns/plugin/executor/hosts"
	_ "github.com/rnetx/cdns/plugin/executor/memcache"
	_ "github.com/rnetx/cdns/plugin/executor/rediscache"
	_ "github.com/rnetx/cdns/plugin/executor/script"
)

func Do() {}
