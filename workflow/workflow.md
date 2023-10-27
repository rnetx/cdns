# Workflow 处理流程

```Workflow``` 是 ```cdns``` 的核心，用户可以自定义 ```Workflow``` 的处理流程，```cdns``` 会按照用户定义的流程处理请求

格式示例：
```yaml
workflows:
    - tag: default
      rules:
        - match-and:
            ...
          exec:
            ...

        - match-or:
            ...
          exec:
            ...

        - exec:
            ...

```

```Workflow``` 基于逻辑处理机制，类似于 ```if-else```，```Workflow``` 定义了三种逻辑处理机制：

```
match-and:          |   match-or:           |   exec:
    array[匹配器]   |       array[匹配器]    |       array[执行器]
else:               |   else:               |
    array[执行器]   |       array[执行器]    |
else-exec:          |   else-exec:          |
    array[执行器]   |       array[执行器]    |
```

- ```match-and```：当**所有** ```match-and``` 中的匹配器**都满足**时，执行 ```exec``` 中的执行器，否则执行 ```else-exec``` 中的执行器

- ```match-or```：当**任意一个** ```match-or``` 中的匹配器**满足**时，执行 ```exec``` 中的执行器，否则执行 ```else-exec``` 中的执行器

- ```exec```：执行 ```exec``` 中的执行器

- ```match-and``` / ```match-or``` / ```exec``` / ```else-exec``` 所有匹配器或者执行器是数组类型，并且匹配/执行顺序是数组中的顺序
