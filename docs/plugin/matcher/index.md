# Plugin Matcher 匹配器插件

```yaml
plugin-matchers:
    - tag: plugin # 插件标签
      type: ${type} # 插件类型
      args: ... # 插件参数

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin # 匹配器插件标签
                args: ... # 运行时插件参数
```

目前支持的匹配器插件列表：

- [domain](domain)
- [ip](ip)
- [geosite](geosite)
- [maxminddb](maxminddb)
- [script](script)
