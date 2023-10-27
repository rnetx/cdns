# Listener - HTTP(S|3) 监听器

```yaml
listeners:
    - tag: listener
      type: http
      deal-timeout: 20s # 处理超时时间
      listen: :443 # 监听地址，示例：127.0.0.1:53 [::1]:53 :53(监听[::]:53)
      # real-ip-header: X-Real-IP # 从请求头获取真实 IP 的字段，可选，默认为空，cdns 会自动从 X-Real-IP X-Forwarded-For 中获取真实 IP
      # trust-ip: 127.0.0.1 # 安全选项，可选，填写则只允许从指定 IP 访问读取真实 IP
      # path: /dns-query # 监听路径，可选，默认为 /dns-query
      # use-http3: false # 是否启用 HTTP/3，可选，默认为 false，填写 true 则必填 TLS 相关配置
      # enable-0rtt: false # 是否启用 0-RTT (QUIC)，可选，默认为 false，仅在 use-http3: true 有效
      server-cert-file: /path/to/cert.pem # TLS 证书文件，可选，填写则使用 HTTPS
      server-key-file: /path/to/key.pem # TLS 私钥文件，可选，填写则使用 HTTPS
      # client-ca-file: /path/to/ca.pem # 客户端 CA 证书文件，用于 mTLS，可选，填写则使用 HTTPS
      # client-ca-file: # 支持多文件
      #   - /path/to/ca1.pem
      #   - /path/to/ca2.pem
```
