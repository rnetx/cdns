# MaxmindDB IP 匹配

MaxmindDB 匹配器可以灵活匹配返回的 ```IP```，提供比 ```resp-ip``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: maxminddb
      args:
        path: /path/to/file # maxminddb 文件
        type: sing # MaxmindDB 文件类型，可选 sing | meta | geolite2-country

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
