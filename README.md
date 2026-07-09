# xAgent Connectors

[简体中文](README.zh-CN.md)

This repository is the public release index for xAgent connectors.

Connector binaries are published through GitHub Releases. Connector source code
is currently maintained in the xAgent main repository unless a specific
connector states otherwise.

## Connectors

| Connector | Directory | Release Tag Pattern | Description |
| --- | --- | --- | --- |
| WeChat Connector | [`connectors/wechat`](connectors/wechat) | `wechat-v*` | Connects xAgent with WeChat IM scenarios. |

## Download

Download connector binaries from:

<https://github.com/coffeehc/xagent-connectors/releases>

Each connector uses its own release tag namespace. For example:

```text
wechat-v0.0.1
feishu-v0.0.1
email-v0.0.1
```

This keeps assets for different connectors separated even when they share the
same product version number.

## Verify Artifacts

When a release provides `SHA256SUMS`, verify downloaded files before
installation:

```bash
shasum -a 256 -c SHA256SUMS
```

## Repository Scope

This repository stores connector release metadata, manifests, installation
notes, and release assets. It should not be used as a source repository for
xAgent connector implementation code unless that boundary changes explicitly.
