# UDP

```yaml
upstreams:
    - tag: upstream
      type: udp
      address: 223.5.5.5 # 服务器地址，支持域名|域名:端口|IP|IP:端口，若服务器地址是域名，必须设置 bootstrap 或（和）socks5
      # connect-timeout: 30s # 连接超时时间
      # idle-timeout: 60s # 连接空闲超时时间
      # enable-pipeline: false # 是否启用 Pipeline (TCP)
      # bootstrap: # 当 address 是域名时，使用 bootstrap 中的上游服务器解析域名
        # upstream: bootstrap-upstream # 上游服务器标签
        # strategy: '' # 解析策略，可选 prefer-ipv4 | prefer-ipv6 | only-ipv4 | only-ipv6 ，默认为 prefer-ipv4
      # bind-interface: eth0 # 绑定网卡
      # bind-ipv4: 0.0.0.0 # 绑定本地 IPv4 地址
      # bind-ipv6: :: # 绑定本地 IPv6 地址
      # so-mark: 255 # 设置 SO_MARK (Linux)
      # socks5: # 使用 SOCKS5 代理
      #   address: 127.0.0.1:1080 # SOCKS5 服务器地址，格式：IP:端口
      #   username: '' # SOCKS5 用户名
      #   password: '' # SOCKS5 密码
```
