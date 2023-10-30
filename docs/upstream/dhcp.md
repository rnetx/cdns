# DHCP

```yaml
upstreams:
    - tag: upstream
      type: dhcp
      interface: eth0 # 绑定的网卡
      # flush-interval: 0s # 自动更新时间间隔
      #
      # 以下配置是创建 UDP DNS 服务器时使用配置
      #
      # connect-timeout: 30s # 连接超时时间
      # idle-timeout: 60s # 连接空闲超时时间
      # edns0: false # 启用 EDNS0 支持，详情参考 https://github.com/IrineSistiana/udpme
      # enable-pipeline: false # 是否启用 Pipeline (TCP)
      # bind-interface: eth0 # 绑定网卡
      # bind-ipv4: 0.0.0.0 # 绑定本地 IPv4 地址
      # bind-ipv6: :: # 绑定本地 IPv6 地址
      # so-mark: 255 # 设置 SO_MARK (Linux)
      # socks5: # 使用 SOCKS5 代理
      #   address: 127.0.0.1:1080 # SOCKS5 服务器地址，格式：IP:端口
      #   username: '' # SOCKS5 用户名
      #   password: '' # SOCKS5 密码
```
