# QUIC

```yaml
listeners:
    - tag: listener
      type: quic
      deal-timeout: 20s # 处理超时时间
      listen: :853 # 监听地址，示例：127.0.0.1:53 [::1]:53 :53(监听[::]:53)
      idle-timeout: 60s # 连接空闲超时时间
      enable-0rtt: false # 是否启用 0-RTT (QUIC)
      server-cert-file: /path/to/cert.pem # TLS 证书文件
      server-key-file: /path/to/key.pem # TLS 私钥文件
      # client-ca-file: /path/to/ca.pem # 客户端 CA 证书文件，用于 mTLS
      # client-ca-file: # 支持多文件
      #   - /path/to/ca1.pem
      #   - /path/to/ca2.pem
```
