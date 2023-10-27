# Parallel

请求并发发送到上游服务器，取最先返回的结果

```yaml
upstreams:
    - tag: upstream
      type: parallel
      upstreams:
        - upstream-a
        - upstream-b
        - upstream-c
```
