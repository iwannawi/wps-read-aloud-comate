# Git 与 GitHub 版本管理规范

## 管理范围

源码仓库提交加载项源代码、服务端源代码、打包脚本、配置模板、说明文档和许可证文件。

普通 Git 提交不直接纳入以下大文件：
- “dist/” 发布安装包和构建出的二进制文件
- “engines/” 离线语音引擎
- “voices/” 语音模型
- “tools/” 本地工具链
- “build/”、“.gocache*”、“downloads/” 等缓存目录

这些大文件应放到 GitHub Release 附件、组织制品库或受控内网文件服务器。

## 分支策略

- “main”：稳定交付分支，只合入已验证版本。
- 临时开发分支仅用于隔离风险，发布前合回 “main”。
- 当前远端仓库应只保留 “main” 作为长期分支。

## 版本号与标签

版本号遵循 “主版本.次版本.修订号”，例如 “1.0.35”。

发布标签格式：

    v1.0.35-20260520

标签日期使用实际发布日。

## Codex 自动发布流程

后续每次修改由 Codex 自动执行以下流程，用户不需要手动操作：

1. 审查变更范围。
2. 更新版本号、发布日期和中文说明文件。
3. 同步加载项文件到服务端内嵌目录。
4. 执行代码检查、XML 检查和打包检查；只有修改 daemon 代码或首次缺少可复用 daemon 二进制时，才执行 Go 编译。
5. 使用 “python packaging/build_all.py” 构建五类安装包，不允许只构建单个安装包后发布版本。
6. 计算并记录五类安装包的 SHA256。
7. 提交到 Git。
8. 使用 “scripts/push_github.ps1” 推送 “main” 和发布标签。
9. 创建发布标签。
10. 使用 “scripts/publish_github_release.ps1” 在 GitHub 创建同名 Release。
11. 在 Release 说明里写入版本变更、已知限制和 SHA256，并上传五类安装包及其 “.sha256” 附件。

## 可复用发布脚本

GitHub 推送和 Release 发布使用仓库内的常驻脚本，不再为每个版本生成临时脚本：

    .\scripts\push_github.ps1 -Tag v1.0.35-20260520
    .\scripts\publish_github_release.ps1 -Version 1.0.35 -ReleaseDate 20260520

脚本使用 HTTPS 认证，不使用 SSH。认证优先级如下：

1. 优先读取 GitHub CLI 的 “gh auth token”。
2. 如果 GitHub CLI 未登录，则读取本机 Git Credential Manager 中保存的 GitHub 凭据。
3. 只有两者都不可用时，才使用安全输入框提示输入 token。

脚本会在使用 token 前调用 GitHub API，并用同一 token 执行 “git ls-remote” 做 HTTPS Git 访问校验；如果发现 “gh” 或 Git Credential Manager 中保存的 token 不能访问当前仓库，会跳过该凭据并提示重新输入。通过安全输入框输入的新 token 会写回 Git Credential Manager，后续推送和 Release 发布应自动读取本机凭据，不再反复弹出 GitHub 认证窗口。

脚本日志不会输出 token 或 Basic 认证头。完成一次 “gh auth login” 或 Git Credential Manager 登录后，后续推送和 Release 发布应自动读取本机凭据。

## 前端小改打包

当前版本中，服务版本号由安装目录中的 “version.json” 提供。仅修改加载项前端、图标、弹窗样式或说明文件时，不需要重新编译 Go 服务；打包脚本会从当前 “dist” 中对应架构的 daemon 二进制或最近一个已生成的 “.deb” 中复用 daemon 二进制。

当前交付包命名示例：

    wps-read-aloud-comate_1.0.35_windows.exe
    wps-read-aloud-comate_1.0.35_amd64.deb
    wps-read-aloud-comate_1.0.35_arm64.deb
    cn.wps-read-aloud-comate_1.0.35_amd64.deb
    cn.wps-read-aloud-comate_1.0.35_arm64.deb
