# 示例配置

```yaml
log:
  level: info

api:
  listen: 127.0.0.1:9088
  secret: admin

upstreams:
  - tag: AliDNS
    type: tls
    address: 223.5.5.5

  - tag: OpenDNS
    type: tls
    address: 208.67.222.222

  - tag: GoogleDNS
    type: tls
    address: 8.8.8.8

  - tag: WorldDNS # 并发请求，防止应某些原因请求失败
    type: parallel
    upstreams:
      - OpenDNS
      - GoogleDNS

plugin-matchers:
  - tag: geosite
    type: sing-geosite
    args:
      path: /etc/cdns/geosite.db # 填写 sing-geosite 位置
      code: gfw

plugin-executors:
  - tag: cache
    type: memcache
    args:
      dump-file: /tmp/dns.cache
      dump-interval: 10s

workflows:
  - tag: main
    rules:
      - exec: # 若命中缓存，则直接返回缓存结果
          - plugin:
              tag: cache
              args:
                mode: restore
                return: true

      - match-or: # 屏蔽 AAAA 和 HTTPS 请求
          - qtype:
              - 28 # AAAA
              - HTTPS
        exec:
          - return: success # 生成空请求

      - match-or: # GFW 列表采用 WorldDNS 查询，否则采用 AliDNS 查询
          - plugin:
              tag: geosite
              args: gfw
        exec:
          - upstream: WorldDNS
        else-exec:
          - upstream: AliDNS

      - exec:
          - plugin: # 缓存结果
              tag: cache
              args:
                mode: store


listeners:
  - tag: listener-tcp
    type: tcp
    listen: :53
    workflow: main

  - tag: listener-udp
    type: udp
    listen: :53
    workflow: main

```
