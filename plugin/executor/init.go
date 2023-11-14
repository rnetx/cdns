package executor

import (
	_ "github.com/rnetx/cdns/plugin/executor/ecs"
	_ "github.com/rnetx/cdns/plugin/executor/ipset"
	_ "github.com/rnetx/cdns/plugin/executor/memcache"
	_ "github.com/rnetx/cdns/plugin/executor/rdns"
	_ "github.com/rnetx/cdns/plugin/executor/rediscache"
	_ "github.com/rnetx/cdns/plugin/executor/script"
)

func Do() {}
