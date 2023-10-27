# Script 脚本

Script 脚本可以触发脚本运行

Script 会将当前请求各种信息存储环境变量中

支持的信息：

- ```CDNS_ID``` ==> 请求的 ID
- ```CDNS_INIT_TIME``` ==> 请求初始化的时间
- ```CDNS_LISTENER``` ==> 请求来源的监听器标签
- ```CDNS_CLIENT_IP``` ==> 请求来源的 ```IP```
- ```CDNS_REQ_QNAME``` ==> 请求的 ```QName```
- ```CDNS_REQ_QTYPE``` ==> 请求的 ```QType```，例如：AAAA
- ```CDNS_REQ_QCLASS``` ==> 请求的 ```QClass```，例如：IN
- ```CDNS_RESP_IP_LEN``` ==> 返回的 ```IP``` 个数，仅当请求类型为 A | AAAA 时有效
- ```CDNS_RESP_IP_${i}``` ==> 返回的 ```IP```，从 1 开始
- ```CDNS_RESP_UPSTREAM_TAG``` ==> 请求的上游服务器标签
- ```CDNS_MARK``` ==> 请求上下文的 ```Mark```
- ```CDNS_METADATA_${KEY}``` ==> 请求上下文的 ```Metadata```，字符串全部大写

---

- Script 还支持替换 ```args``` 中的字符串，只需要在 ```args``` 设置 {KEY} 即可

```yaml
plugin-executors:
    - tag: plugin
      type: script
      args:
        command: bash
        # args: '-c'
        # args:
        #   - '-c'
        #   - '-b'

workflows:
    - tag: default
      rules:
        - exec:
            - plugin:
                tag: plugin
```
