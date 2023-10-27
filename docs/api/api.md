# API 服务器

```API``` 服务器支持暴露 HTTP 接口，提供更多功能

```yaml
api:
    listen: 127.0.0.1:8099 # HTTP 监听地址
    secret: admin # 鉴权密码，需设置 Header: Authorization Bearer ${secret}
    debug: false # 开启 pprof
```

路径：

- ```/debug``` ==> pprof 路径，只有在 debug: true 监听
- ```/upstream``` ==> 获取所有 Upstream API
- ```/upstream/${upstream-tag}``` ==> 获取 Upstream API 信息
- ```/plugin/matcher``` ==> 获取所有 Plugin Matcher API
- ```/plugin/matcher/${plugin-matcher-tag}``` ==> 获取 Plugin Matcher API 信息
- ```/plugin/matcher/${plugin-matcher-tag}/help``` ==> 获取 Plugin Matcher API 所有接口信息
- ```/plugin/executor``` ==> 获取所有 Plugin Executor API
- ```/plugin/executor/${plugin-executor-tag}``` ==> 获取 Plugin Executor API 信息
- ```/plugin/executor/${plugin-executor-tag}/help``` ==> 获取 Plugin Executor API 所有接口信息

### Upstream API

GET /upstream

返回值：
```json5
{
    "data": {
        "${upstream-tag}": {
            "tag": "${upstream-tag}",
            "type": "${upstream-type}",
            "data": {
                "total": 0, # 总请求数
                "success": 0 # 成功请求数
            }
        }
    }
}
```

GET /upstream/${upstream-tag}

返回值：
```json5
{
    "tag": "${upstream-tag}",
    "type": "${upstream-type}",
    "data": {
        "total": 0, # 总请求数
        "success": 0 # 成功请求数
    }
}
```

### Plugin Matcher API

GET /plugin/matcher

返回值：
```json5
{
    "data": {
        "${plugin-matcher-tag}": {
            "tag": "${plugin-matcher-tag}",
            "type": "${plugin-matcher-type}"
        }
    }
}
```

GET /plugin/matcher/${plugin-matcher-tag}

返回值：
```json5
{
    "tag": "${plugin-matcher-tag}",
    "type": "${plugin-matcher-type}"
}
```

GET /plugin/matcher/${plugin-matcher-tag}/help

返回值：
```json5
{
    "/${path}": {
        "methods": [...], // GET POST ...
        "description": ... // API 介绍
    }
}
```

### Plugin Executor API

GET /plugin/executor

返回值：
```json5
{
    "data": {
        "${plugin-executor-tag}": {
            "tag": "${plugin-executor-tag}",
            "type": "${plugin-executor-type}"
        }
    }
}
```

GET /plugin/executor/${plugin-executor-tag}

返回值：
```json5
{
    "tag": "${plugin-executor-tag}",
    "type": "${plugin-executor-type}"
}
```

GET /plugin/executor/${plugin-executor-tag}/help

返回值：
```json5
{
    "/${path}": {
        "methods": [...], // GET POST ...
        "description": ... // API 介绍
    }
}
```
