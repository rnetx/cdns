# UDP

```yaml
listeners:
    - tag: listener
      type: udp
      deal-timeout: 20s # 处理超时时间
      listen: :6053 # 监听地址，示例：127.0.0.1:53 [::1]:53 :53(监听[::]:53)

```
