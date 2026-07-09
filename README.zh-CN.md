# xAgent Connectors

[English](README.md)

本仓库是 xAgent 连接器的公开发布索引仓库。

连接器二进制文件通过 GitHub Releases 发布。除非某个连接器另有说明，连接器源码目前仍维护在 xAgent 主仓库中。

使用文档：

<https://xagent.xiagaogao.com>

## 连接器列表

| 连接器 | 目录 | Release Tag 规则 | 说明 |
| --- | --- | --- | --- |
| WeChat Connector | [`connectors/wechat`](connectors/wechat) | `v0.0.1` | 用于将 xAgent 接入微信 IM 场景。 |

## 下载

请从 GitHub Releases 页面下载连接器二进制文件：

<https://github.com/coffeehc/xagent-connectors/releases>

当前微信连接器发布使用：

```text
v0.0.1
```

后续如果多个连接器需要独立发布，可以再使用按连接器区分的 tag 命名。

## 校验

如果 Release 提供 `SHA256SUMS`，建议安装前先校验下载文件：

```bash
shasum -a 256 -c SHA256SUMS
```

## 仓库边界

本仓库保存连接器发布元信息、manifest、安装说明和 Release 附件。除非后续明确调整边界，否则不要把 xAgent 连接器实现源码放在本仓库中。
