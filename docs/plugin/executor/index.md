# Plugin Executor 执行器插件

```yaml
plugin-executors:
    - tag: plugin # 插件标签
      type: ${type} # 插件类型
      args: ... # 插件参数

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin # 执行器插件标签
                args: ... # 运行时插件参数
```

目前支持的匹配器插件列表：

- [memcache](memcache)
- [rediscache](rediscache)
- [script](script)
- [ecs](ecs)
