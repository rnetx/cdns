# Upstream - QueryTest 上游服务器

测试所有上游服务器，并选择延迟最低的上游服务器

```yaml
upstreams:
    - tag: upstream
      type: querytest
      upstreams:
        - upstream-a
        - upstream-b
        - upstream-c
      test-domain: www.example.com # 测试域名
      test-interval: 600s # 测试间隔时间
      tolerance: 3ms # 比当前最佳上游服务器延迟低超过 tolerance 才会选择
```
