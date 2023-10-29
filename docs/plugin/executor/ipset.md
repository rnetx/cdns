# IPSet

IPSet 可以将 ```IP``` 添加到 IPSet，仅支持 Linux

```yaml
plugin-executors:
    - tag: plugin
      type: ipset
      args:
        name4: set4 # IPv4 IPSet 名称
        name6: set6 # IPv6 IPSet 名称
        mask4: 32 # IPv4 IPSet 掩码
        mask6: 128 # IPv6 IPSet 掩码
        ttl4: 600s # IPv4 IPSet TTL
        ttl6: 600s # IPv6 IPSet TTL
        create4: false # 是否在启动时创建
        create6: false # 是否在启动时创建
        destroy4: false # 是否在停止时销毁
        destroy6: false # 是否在停止时销毁

workflows:
    - tag: default
      rules:
        - exec:
            - plugin:
                tag: plugin
                # args:
                #   use-client-ip: false # 使用客户端 IP，而非 DNS 返回的 IP
```

### API

GET /flush

清空 IPSet

返回状态：204
