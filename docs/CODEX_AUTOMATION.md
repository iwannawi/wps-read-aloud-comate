# Codex 自动执行规范

本项目由 Codex 自动完成本地修改、构建、Git 推送和 GitHub Release 发布。

## 默认流程

1. 查看 Git 状态。
2. 判断影响范围。
3. 修改代码、脚本、配置或说明文件。
4. 执行必要检查。
5. 确认构建产物和缓存未进入源码提交。
6. 提交 Git。
7. 推送 main。
8. 如形成正式版本，创建并推送标签。
9. 发布 GitHub Release。

## 普通修改

普通修改默认提交并推送，不创建标签。

普通修改包括：

- 文档更新。
- UI 文案或图标微调。
- 脚本兼容性小修。
- 错误提示优化。
- 不改变交付包内容的小修复。

## 正式版本

以下情况视为正式版本：

- 用户要求重新交付安装包。
- 修改安装包内容。
- 修改 WPS 加载项运行逻辑。
- 修改本地朗读服务。
- 修改安装、卸载、注册或 systemd 脚本。
- 修改第三方依赖、模型、引擎或许可证说明。

正式版本必须构建五类安装包：

| CPU 架构 + 操作系统 | 安装包 |
| --- | --- |
| x86/x64 Windows 10/11 | exe |
| x64 银河麒麟系统 | deb |
| ARM64 银河麒麟系统 | deb |
| x64 UOS系统 | deb |
| ARM64 UOS系统 | deb |

## GitHub 管理

远程仓库：

    https://github.com/iwannawi/wps-read-aloud-comate

默认分支：

    main

源码进入 Git。安装包、语音模型、离线引擎、工具链和构建缓存不进入普通 Git。

## 安全边界

Codex 不会把 Personal Access Token 写入：

- Git remote URL。
- .git/config。
- 项目源码。
- 文档。
- 日志。
- GitHub Actions 工作流明文。

本项目使用 HTTPS 访问 GitHub，不使用 SSH。推送和 Release 发布脚本优先读取 GitHub CLI，其次读取 Git Credential Manager。两者都不可用时才提示输入 token。

## 暂停确认

以下操作需要暂停确认：

- 删除用户数据或不可恢复文件。
- 强制覆盖远程历史。
- 修改、暴露或保存新的密钥、token、证书。
- 清理非项目目录。
- 与当前需求无关的大规模重构。

其他明确需求由 Codex 默认执行到提交、推送和发布完成。
