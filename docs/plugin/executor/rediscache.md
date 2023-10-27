# RedisCache 缓存

RedisCache 缓存可以缓存返回的结果，提高性能，使用 Redis 作为存储机制

```yaml
plugin-executors:
    - tag: plugin
      type: rediscache
      args:
        address: 127.0.0.1:6379 # Redis 地址，支持 Unix Socket
        password: '' # Redis 密码
        db: 0 # Redis DB

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

GET | DELETE /flush

删除所有 Redis 中的缓存

返回状态：204
