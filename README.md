# cdns

```cdns``` 是一个使用 Golang 编写的，高度自定义的 DNS 服务器

---

### 如何构建
```
make build
```
- Release 默认包含所有插件
- 如果想去除不需用到的插件，可以编辑 ```plugin/matcher/init.go``` 或 ```plugin/executor/init.go``` 文件
```
    plugin/matcher/init.go 在不需要的匹配插件前加 “//”
    plugin/executor/init.go 在不需要的执行插件前加 “//”
    例如：
    _ "path/to/plugin-need"
    // _ "path/to/plugin-unneed"
```

## 开源许可证

cdns 使用 GPL-3.0 开源许可证，详细请参阅 [LICENSE](LICENSE) 文件。
