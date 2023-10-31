# NTP 服务器

当本地服务器因某些原因无法校准正确的时间，而 ```Upstream``` 中含有使用 ```TLS``` 的 ```Upstream``` ，在建立连接时会因为时间不准确而失败，可以通过 NTP 服务器来校准时间。

```yaml
ntp:
  server: ntp.aliyun.com # NTP 服务器地址，支持域名|域名:端口|IP|IP:端口，若服务器地址是域名，必须设置 upstream
  # interval: 600s # 校准时间间隔，默认 30min
  # upstream: upstream # 上游服务器标签，用于解析 NTP 服务器地址，仅支持 TCP/UDP ，或者不需要依赖（间接依赖）TLS 的其他上游服务器，配置错误会导致回环，谨慎配置！
  # write-to-system: false # 是否将时间写入系统中，仅支持 Unix 或 Windows 系统，可能需要系统高级权限
  #
  # 以下配置用于 NTP 请求，可选
  #
  # bind-interface: eth0 # 绑定网卡
  # bind-ipv4: 0.0.0.0 # 绑定本地 IPv4 地址
  # bind-ipv6: :: # 绑定本地 IPv6 地址
  # so-mark: 255 # 设置 SO_MARK (Linux)
```
