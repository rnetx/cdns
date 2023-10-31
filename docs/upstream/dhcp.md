# DHCP

```yaml
upstreams:
    - tag: upstream
      type: dhcp
      interface: eth0 # 绑定的网卡，留空自动选择，可能会失败
      #
      # 以下配置是创建 UDP DNS 服务器时使用配置
      #
      # connect-timeout: 30s # 连接超时时间
      # idle-timeout: 60s # 连接空闲超时时间
      # edns0: false # 启用 EDNS0 支持，详情参考 https://github.com/IrineSistiana/udpme
      # enable-pipeline: false # 是否启用 Pipeline (TCP)
```
