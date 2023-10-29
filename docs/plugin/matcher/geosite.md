# GeoSite 域名匹配

GeoSite 匹配器可以灵活匹配域名，提供比 ```qname``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: geosite
      args:
        path: /path/to/file # 规则文件
        type: sing # geosite 文件类型，必填，可选 sing | meta | v2ray
        code: cn # 载入的标签，为空载入所有标签，这会增加内存占用，只当 type: sing | v2ray 生效
        # code: # 载入多个标签
        #   - cn
        #   - google

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin
                args: cn # 匹配的标签，只当 type: sing | v2ray 生效
                # args: cn,google # 多个匹配的标签
                # args: # 多个匹配的标签
                #   - cn
                #   - google,netflix
                # args: # 这样也可以
                #   code: cn
                # args: # 这样也可以
                #   code:
                #     - cn
                #     - google
          exec:
            ...
```

### API

GET /reload

重新加载规则文件

返回状态：204

### 注意

Release 构建的二进制文件默认包含三种不同类型的 ```geosite```，文件体积可能很大，你可以使用 ```UPX``` 工具压缩。如果文件大小依然无法接受，可以自行编译去除不需要的 ```geosite``` 类型。

方法：在 ```plugin/matcher/geosite/geosite.go``` 中修改 ```import``` 即可

```go
// 取消前面的注释即可
import (
	"github.com/rnetx/cdns/plugin/matcher/geosite/meta" // 添加 meta 格式支持
	// meta "github.com/rnetx/cdns/plugin/matcher/geosite/meta_stub" // 不添加 meta 格式支持

	"github.com/rnetx/cdns/plugin/matcher/geosite/sing" // 添加 sing 格式支持
	// sing "github.com/rnetx/cdns/plugin/matcher/geosite/sing_stub" // 不添加 sing 格式支持

	"github.com/rnetx/cdns/plugin/matcher/geosite/v2xray" // 添加 v2ray 格式支持
	// v2xray "github.com/rnetx/cdns/plugin/matcher/geosite/v2xray_stub" // 不添加 v2ray 格式支持
)
```
