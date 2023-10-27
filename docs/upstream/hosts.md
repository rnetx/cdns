# Hosts

根据规则返回指定 IPv4 / IPv6 地址，仅支持 (A | AAAA) 请求。没有规则匹配的请求或者非 (A | AAAA) 请求将发送到 fallback 上游服务器

```yaml
upstreams:
    - tag: upstream
      type: hosts
      fallback: upstream-fallback # 没有规则匹配的请求或者非 (A | AAAA) 请求将发送到 fallback 上游服务器
      rule: # 规则，键值对(正则表达式字符串 => IP / CIDR)
        '^example.*': 192.168.1.1
        'cloudflare': # 可以设置多个地址
          - 1.1.1.1
          - 1.0.0.0/24 # 支持 CIDR ，会随机从这个范围中选择一个
```
