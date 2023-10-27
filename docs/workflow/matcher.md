# Workflow - Matcher 匹配器

下面是一些 ```Workflow``` 预定义的匹配器

- [```Listener```](#listener)
- [```Client-IP```](#client-ip)
- [```QType```](#qtype)
- [```QName```](#qname)
- [```Has-Resp-Msg```](#has-resp-msg)
- [```Resp-IP```](#resp-ip)
- [```Mark```](#mark)
- [```Env```](#env)
- [```Metadata```](#metadata)
- [```Plugin```](#plugin)
- [```Match-And```](#match-and)
- [```Match-Or```](#match-or)
- [```Invert``` *](#invert)

### ```Listener```

匹配请求的监听器

值类型：字符串 | 数组(字符串)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - listener: listener-tcp
            - listener: # 这样也可以，只需一个匹配即可
                - listener-tcp
                - listener-udp
          exec:
            ...
```

### ```Client-IP```

匹配请求的客户端 IP

值类型：(```IP``` / ```CIDR```) | 数组(```IP``` / ```CIDR```)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - client-ip: 192.168.0.2
            - client-ip: # 这样也可以，只需一个匹配即可
                - 192.168.0.4
                - 192.168.0.0/24 # 这样也可以
          exec:
            ...
```

### ```QType```

匹配请求的查询类型

值类型：字符串 | 数组(字符串) | 正整数 | 数组(正整数)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - qtype: 28 # AAAA
            - qtype: # 这样也可以，只需一个匹配即可
                - 28 # AAAA
                - HTTPS # 这样也可以, 65
          exec:
            ...
```

### ```QName```

匹配请求的查询名

值类型：字符串 | 数组(字符串)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - qname: www.google.com
            - qname: # 这样也可以，只需一个匹配即可
                - www.google.com
                - www.youtube.com
          exec:
            ...
```

### ```Has-Resp-Msg```

匹配请求是否有响应消息

值类型：布尔值

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - has-resp-msg: true # 是否有响应消息
            - has-resp-msg: false # 是否没有响应消息
          exec:
            ...
```

### ```Resp-IP```

匹配响应 IP，所有 ```IP``` 地址只需要匹配一个即可

值类型：(```IP``` / ```CIDR```) | 数组(```IP``` / ```CIDR```)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - resp-ip: 192.168.0.1
            - resp-ip: # 这样也可以，只需一个匹配即可
                - 192.168.0.2
                - 192.168.0.0/24 # 这样也可以
          exec:
            ...
```

### ```Mark```

匹配标记 ```Mark```，所有 ```Mark``` 只需要匹配一个即可

- ```Mark``` 可以通过 ```Mark``` 执行器设置

值类型：正整数 | 数组(正整数)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - mark: 1
            - mark: # 这样也可以，只需一个匹配即可
                - 2
                - 3
          exec:
            ...
```

### ```Env```

匹配环境变量 ```Env```，所有 ```Env``` 只需要匹配一个即可

值类型：键值对(字符串 => 字符串)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - env:
                foo: bar
                foo2: bar2
          exec:
            ...
```

### ```Metadata```

匹配 ```Metadata```，所有 ```Metadata``` 只需要匹配一个即可

- ```Metadata``` 可以通过 ```Metadata``` 执行器设置

值类型：键值对(字符串 => 字符串)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - metadata:
                foo: bar
                foo2: bar2
          exec:
            ...
```

### ```Plugin```

通过匹配器插件匹配请求

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - plugin:
                tag: plugin-tag # 插件标签
                args: # 插件参数，视插件而定
                  foo: bar
          exec:
            ...
```

### ```Match-And```

要求匹配 ```match-and``` 中的**所有**匹配器

值类型：数组(匹配器)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - match-and:
                - ...
                - ...
          exec:
            ...
```

### ```Match-Or```

要求匹配 ```match-or``` 中的**任意一个**匹配器

值类型：数组(匹配器)

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - match-or:
                - ...
                - ...
          exec:
            ...
```

### ```Invert```

反转匹配结果，可以附加在任意匹配器上

```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            - client-ip: 192.168.1.1
              invert: true # 反转匹配结果
            - resp-ip:
                - 192.168.1.3
                - 192.168.1.0/24
              invert: true # 反转匹配结果
          exec:
            ...
```
