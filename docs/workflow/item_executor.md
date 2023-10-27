# Workflow - Executor 执行器

下面是一些 ```Workflow``` 预定义的执行器

- ```Mark```
- ```Metadata```
- ```Plugin```
- ```Upstream```
- ```Jump-To```
- ```Go-To```
- ```Workflow-Rules```
- ```Fallback```
- ```Parallel```
- ```Set-TTL```
- ```Set-Resp-IP```
- ```Clean```
- ```Return```

### ```Mark```

设置 ```Mark```

值类型：非负整数

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - mark: 1 # 设置 Mark 为 1
            - mark: 0 # 删除 Mark
```

### ```Metadata```

设置 ```Metadata```，所有 ```Metadata``` 都会被设置

值类型：键值对(字符串 => 字符串)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - metadata:
                foo: bar # 设置 Metadata foo 为 bar
                foo2: '' # 删除 Metadata foo2
```

### ```Plugin```

通过执行器插件执行

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - plugin:
                tag: plugin-tag # 插件标签
                args: # 插件参数，视插件而定
                  foo: bar
```

### ```Upstream```

向 ```Upstream``` 发送请求并获得响应

值类型：字符串

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - upstream: upstream-default
            - upstream:
                tag: upstream-default
                strategy: prefer-ipv4 # 若请求为 AAAA ，则同时请求 A ，若 A 返回有效响应，则忽略 AAAA 的响应，生成空响应 (Rcode: Success) (SOA)
                # strategy: prefer-ipv6 # 若请求为 A ，则同时请求 AAAA ，若 AAAA 返回有效响应，则忽略 A 的响应，生成空响应 (Rcode: Success) (SOA)
```

### ```Jump-To```

跳转到指定的 ```Workflow``` 处理，处理结束后返回到当前 ```Workflow``` 继续处理

值类型：字符串 | 数组(字符串)

```yaml
workflows:
    - tag: w1
      rules:
        ...

    - tag: w2
      rules:
        ...

    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - jump-to: w1
            - jump-to: # 顺序执行 w1 和 w2
                - w1
                - w2
```

### ```Go-To```

跳转到指定的 ```Workflow``` 处理，处理结束后不会返回到当前 ```Workflow``` 继续处理

值类型：字符串

```yaml
workflows:
    - tag: w1
      rules:
        ...

    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - go-to: w1
```

### ```Workflow-Rules```

内置的 ```Workflow``` 处理逻辑，用于在 ```Workflow``` 中嵌套 ```Workflow``` 处理

值类型：数组(match-and | match-or | exec)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - workflow-rules:
                - match-or:
                    ...
                  exec:
                    ...

                - match-and:
                    ...
                  exec:
                    ...
```

### ```Fallback```

设置主-从逻辑，当主逻辑处理失败时，会尝试从逻辑处理

- 主从逻辑会克隆原上下文，并在处理成功后改写原上下文，主从逻辑上下文**不**相互冲突

```yaml
workflows:
    - tag: w1
      rules:
        ...

    - tag: w2
      rules:
        ...

    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - fallback:
                main: # 与 main-workflow 两者只能设置一个
                    - match-and:
                        ...
                      exec:
                        ...
                    ...
                main-workflow: w1 # 与 main 两者只能设置一个
                fallback: # 与 fallback-workflow 两者只能设置一个
                    - match-and:
                        ...
                      exec:
                        ...
                    ...
                fallback-workflow: w2 # 与 fallback 两者只能设置一个
                wait-time: 0 # 从逻辑启动等待时间
```

### ```Parallel```

并行执行多个处理流程，取最快的一个作为结果

- 新处理流程会克隆原上下文，并在处理成功后改写原上下文，所有处理流程上下文**不**相互冲突

```yaml
workflows:
    - tag: w1
      rules:
        ...

    - tag: w2
      rules:
        ...

    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - parallel:
                workflows:
                  - w1
                  - w2
```

### ```Set-TTL```

设置响应的 ```TTL```

值类型：非负整数

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - set-ttl: 3600 # 设置 TTL 为 3600 秒
```

### ```Set-Resp-IP```

设置响应的 ```IP```

值类型：(```IP``` / ```CIDR```) | 数组(```IP``` / ```CIDR```)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - set-resp-ip: 192.168.1.1 # 只对 A 请求有效
            - set-resp-ip: # 这样也可以，会附加所有有效的 IP
                - 192.168.1.1 # 只对 A 请求有效
                - 192.168.1.0/24 # 只对 A 请求有效，且会在 CIDR 中随机挑选一个 IP
            - set-resp-ip: fd00::1 # 只对 AAAA 请求有效
            - set-resp-ip: # 这样也可以，会附加所有有效的 IP
                - fd00::1 # 只对 AAAA 请求有效
                - fd00::1/60 # 只对 AAAA 请求有效，且会在 CIDR 中随机挑选一个 IP
```

### ```Clean```

清理响应信息

值类型：布尔值 | 无

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - clean: true
            - clean: false # 无效
            - clean # 这样也可以，等效于 - clean: true
```

### ```Return```

退出处理流程，返回响应

值类型：布尔值 | 字符串 | 无

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            - return: true # 退出所有处理流程，直接返回
            - return: false # 无效
            - return # 这样也可以，等效于 - return: true
            - return: all # 退出所有处理流程，直接返回，等效于 - return: true
            - return: once # 退出当前处理流程，返回到上一个处理流程
            - return: success # 退出所有处理流程，并生成响应(Rcode: Success)
            - return: failure # 退出所有处理流程，并生成响应(Rcode: Failure)
            - return: nxdomain # 退出所有处理流程，并生成响应(Rcode: Nxdomain)
            - return: refused # 退出所有处理流程，并生成响应(Rcode: Refused)
```
