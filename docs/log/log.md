# Log 日志

```yaml
log:
    disabled: false # 是否禁用日志输出
    level: info # 日志等级，可选 debug | info | warn | error | fatal
    output: /path/to/file.log # 日志文件，可选 stdout：标准输出 ，stderr：错误输出
    disable-timestamp: false # 禁用时间戳信息
    disable-color: false # 禁用颜色输出，当 output 为文件时默认禁用
```
