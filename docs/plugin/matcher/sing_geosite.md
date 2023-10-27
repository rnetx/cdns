# Sing-GeoSite 域名匹配

Sing-GeoSite 匹配器可以灵活匹配域名，提供比 ```qname``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: sing-geosite
      args:
        path: /path/to/file # 规则文件
        code: cn # 载入的标签，为空载入所有标签，这会增加内存占用
        # code: # 载入多个标签
        #   - cn
        #   - google

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin
                args: cn # 匹配的标签
                # args: cn,google # 多个匹配的标签
                # args: # 多个匹配的标签
                #   - cn
                #   - google,netflix
          exec:
            ...
```

### API

GET /reload

重新加载规则文件

返回状态：204
