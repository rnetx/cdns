package matcher

import (
	_ "github.com/rnetx/cdns/plugin/matcher/domain"
	_ "github.com/rnetx/cdns/plugin/matcher/geosite"
	_ "github.com/rnetx/cdns/plugin/matcher/ip"
	_ "github.com/rnetx/cdns/plugin/matcher/maxminddb"
	_ "github.com/rnetx/cdns/plugin/matcher/script"
)

func Do() {}
