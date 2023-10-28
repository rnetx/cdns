# GeoSite 域名匹配

GeoSite 匹配器可以灵活匹配域名，提供比 ```qname``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: geosite
      args:
        path: /path/to/file # 规则文件
        type: sing # geosite 文件类型，必填，可选 sing | meta
        code: cn # 载入的标签，为空载入所有标签，这会增加内存占用，只当 type: sing 生效
        # code: # 载入多个标签
        #   - cn
        #   - google

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin
                args: cn # 匹配的标签，只当 type: sing 生效
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
