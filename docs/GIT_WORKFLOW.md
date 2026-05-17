# Git 与 GitHub 版本管理规范

## 管理范围

源码仓库提交加载项源代码、服务端源代码、打包脚本、配置模板、说明文档和许可证文件。

普通 Git 提交不直接纳入以下大文件：
- `dist/` 发布安装包和构建出的二进制文件
- `engines/` 离线语音引擎
- `voices/` 语音模型
- `tools/` 本地工具链
- `build/`、`.gocache*`、`downloads/` 等缓存目录

这些大文件应放到 GitHub Release 附件、企业制品库或受控内网文件服务器。

## 分支策略

- `main`：稳定交付分支，只合入已验证版本。
- 临时开发分支仅用于隔离风险，发布前合回 `main`。
- 当前远端仓库应只保留 `main` 作为长期分支。

## 版本号与标签

版本号遵循 `主版本.次版本.修订号`，例如 `1.0.21`。

发布标签格式：

```text
v1.0.21-20260517
```

标签日期使用实际发布日。

## Codex 自动发布流程

后续每次修改由 Codex 自动执行以下流程，用户不需要手动操作：

1. 审查变更范围。
2. 更新版本号、发布日期和中文说明文件。
3. 同步加载项文件到服务端内嵌目录。
4. 执行代码检查、XML 检查和打包检查；只有修改 daemon 代码或首次缺少可复用 daemon 二进制时，才执行 Go 编译。
5. 构建 `wps-read-aloud-xc_<版本>_arm64.deb`。
6. 计算并记录 SHA256。
7. 提交到 Git。
8. 使用 `scripts/push_github.ps1` 推送 `main` 和发布标签。
9. 创建发布标签。
10. 使用 `scripts/publish_github_release.ps1` 在 GitHub 创建同名 Release。
11. 在 Release 说明里写入版本变更、已知限制和 SHA256，并上传 `.deb` 安装包附件。

## 可复用发布脚本

GitHub 推送和 Release 发布使用仓库内的常驻脚本，不再为每个版本生成临时脚本：

```powershell
.\scripts\push_github.ps1 -Tag v1.0.21-20260517
.\scripts\publish_github_release.ps1 -Version 1.0.21 -ReleaseDate 20260517
```

脚本默认从本机 Git Credential Manager 读取 GitHub 凭据。只有本机凭据不可用时，才会使用安全输入框提示输入 token。脚本日志不会输出 token 或 Basic 认证头。

## 前端小改打包

从 `1.0.21` 开始，服务版本号由 `/opt/wps-read-aloud/version.json` 提供。仅修改加载项前端、图标、弹窗样式或说明文件时，不需要重新编译 Go 服务；打包脚本会从当前 `dist/wps-tts-daemon` 或最近一个已生成的 `.deb` 中复用 daemon 二进制。

当前交付包命名示例：

```text
wps-read-aloud-xc_1.0.21_arm64.deb
```
