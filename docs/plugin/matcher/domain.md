# Domain 域名匹配

域名匹配器可以灵活匹配域名，提供比 ```qname``` 更灵活的规则机制

```yaml
plugin-matchers:
    - tag: plugin
      type: domain
      args:
        rule: 'full:google.com' # 匹配规则
        # rule:
        #   - 'full:google.com'
        #   - 'suffix:google.com'
        #   - 'keyword:google'
        #   - 'regex:google'
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
