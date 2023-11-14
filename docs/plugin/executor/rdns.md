# rDNS IP 反查

rDNS 用于查询 IP 的反向域名，通常用于局域网内

```yaml
plugin-executors:
    - tag: plugin
      type: rdns

workflows:
    - tag: default
      rules:
        - exec:
            - plugin:
                tag: plugin
                args: # 键值对：IP|CIDR: 上游服务器 Tag
                  '*': upstream-A # '*' 用于匹配任意 IP
                  '192.168.1.0/24': upstream-Local
                  'fd00::/8': upstream-Local
```
