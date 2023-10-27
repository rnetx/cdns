# Random

随机将请求发送到上游服务器

```yaml
upstreams:
    - tag: upstream
      type: random
      upstreams:
        - upstream-a
        - upstream-b
        - upstream-c
```
