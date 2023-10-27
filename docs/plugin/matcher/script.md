# Script 脚本匹配

脚本匹配器可以根据脚本运行结果进行匹配

脚本需要在标准输出返回并且只能返回以下内容：
```
1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False
```

```yaml
plugin-matchers:
    - tag: plugin
      type: script
      args:
        command: bash
        # args: '-c'
        # args:
        #   - '-b'
        #   - '-b'
        interval: 300s # 脚本执行间隔

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

GET /run

手动执行一遍脚本

返回状态：204

GET /result

获取当前状态

返回值：
```json5
{
    "result": true
}
```
