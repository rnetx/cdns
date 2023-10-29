# ECS

ECS 可以附加 ```edns client subnet``` 记录

```yaml
plugin-executors:
    - tag: plugin
      type: ecs
      args:
        ipv4: 192.168.1.1
        ipv6: 2001:db8::1
        mask4: 24
        mask6: 60

workflows:
    - tag: default
      rules:
        - exec:
            - plugin:
                tag: plugin
```
