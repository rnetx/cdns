# IP 匹配

```IP``` 匹配器可以灵活匹配返回的 ```IP```，提供比 ```resp-ip``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: ip
      args:
        rule: '1.1.1.1' # 匹配规则
        # rule:
        #   - '1.1.1.1'
        #   - '1.0.0.0/24'
        file: /path/to/rule # 规则文件
        # file:
        #   - /path/to/file1
        #   - /path/to/file2

workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin
          exec:
            ...
```

### API

GET /reload

重新加载规则文件

返回状态：204
