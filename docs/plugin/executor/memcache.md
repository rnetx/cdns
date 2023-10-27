# MemCache 缓存

MemCache 缓存可以缓存返回的结果，提高性能

```yaml
plugin-executors:
    - tag: plugin
      type: memcache
      args:
        dump-path: /path/to/rule # 缓存文件，可选
        dump-interval: 0 # 自动缓存时间间隔

workflows:
    - tag: default
      rules:
        - exec:
            - plugin:
                tag: plugin
                args:
                  mode: store # 缓存结果
                  return: true # 缓存成功后，终止所有处理流程，并返回
    
        - exec:
            - plugin:
                tag: plugin
                args:
                  mode: restore # 从缓存获取结果
                  return: true # 获取缓存成功后，终止所有处理流程，并返回
```

### API

GET /dump

将缓存保持到本地文件

返回状态：204

GET | DELETE /flush

删除所有内存中的缓存

返回状态：204
