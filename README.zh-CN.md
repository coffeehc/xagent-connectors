# xAgent Connectors

[English](README.md)

本仓库是 xAgent 连接器的公开发布索引仓库。

连接器二进制文件通过 GitHub Releases 发布。除非某个连接器另有说明，连接器源码目前仍维护在 xAgent 主仓库中。

## 连接器列表

| 连接器 | 目录 | Release Tag 规则 | 说明 |
| --- | --- | --- | --- |
| WeChat Connector | [`connectors/wechat`](connectors/wechat) | `wechat-v*` | 用于将 xAgent 接入微信 IM 场景。 |

## 下载

请从 GitHub Releases 页面下载连接器二进制文件：

<https://github.com/coffeehc/xagent-connectors/releases>

每个连接器使用独立的 Release tag 命名空间，例如：

```text
wechat-v0.0.1
feishu-v0.0.1
email-v0.0.1
```

这样即使多个连接器使用同一个产品版本号，也不会把不同连接器的附件混在同一个 Release 里。

## 校验

如果 Release 提供 `SHA256SUMS`，建议安装前先校验下载文件：

```bash
shasum -a 256 -c SHA256SUMS
```

## 仓库边界

本仓库保存连接器发布元信息、manifest、安装说明和 Release 附件。除非后续明确调整边界，否则不要把 xAgent 连接器实现源码放在本仓库中。
