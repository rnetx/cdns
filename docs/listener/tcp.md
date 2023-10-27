# TCP

```yaml
listeners:
    - tag: listener
      type: tcp
      deal-timeout: 20s # 处理超时时间
      listen: :6053 # 监听地址，示例：127.0.0.1:53 [::1]:53 :53(监听[::]:53)
      idle-timeout: 60s # 连接空闲超时时间

```
