# Git 与 GitHub 版本管理规范

## 项目标识

| 项目 | 值 |
| --- | --- |
| 本地项目名 | wps-read-aloud-comate |
| GitHub 仓库 | iwannawi/wps-read-aloud-comate |
| 默认分支 | main |
| 安装包基础名 | wps-read-aloud-comate |

## 管理范围

Git 保存源码、脚本、配置、文档和许可证。

以下内容不进入普通 Git 提交：

- dist 发布安装包和构建产物。
- engines 离线语音引擎。
- voices 语音模型。
- tools 本地工具链。
- build、downloads、Go 缓存和临时文件。

大文件通过 GitHub Release、组织制品库或受控内网文件服务器交付。

## 分支策略

- main 是稳定交付分支。
- 临时开发分支只用于隔离风险，发布前合回 main。
- 远端仓库长期只保留 main。

## 版本号规则

版本号采用“主版本.次版本.修订号”。

| 类型 | 触发条件 | 示例 |
| --- | --- | --- |
| 主版本 | 架构重构、安装方式不兼容、核心能力大幅改变 | 2.0.0 |
| 次版本 | 新平台、新安装包类型、重要能力升级、交付规范升级 | 1.1.9 |
| 修订号 | 缺陷修复、文案优化、兼容性小改 | 1.1.9 |

本项目从 1.1.9 开始按此规则发布。标签格式为：

    v1.1.9-20260523

Release 名称格式为：

    wps-read-aloud-comate 1.1.9 20260523

## 自动发布流程

正式版本由 Codex 自动执行：

1. 审查变更范围。
2. 更新版本号、发布日期和发布说明。
3. 同步 addin 到 Go embed 目录。
4. 执行语法检查、代码检查和打包检查。
5. 构建五类安装包。
6. 计算 SHA256。
7. 提交 Git。
8. 推送 main 和版本标签。
9. 创建 GitHub Release。
10. 上传五类安装包和校验文件。

## 发布脚本

推送：

    .\scripts\push_github.ps1 -Tag v1.1.9-20260523

发布 Release：

    .\scripts\publish_github_release.ps1 -Version 1.1.9 -ReleaseDate 20260523

脚本使用 HTTPS，不使用 SSH。认证优先读取 GitHub CLI，其次读取 Git Credential Manager。脚本不得把 token 写入源码、日志或 Git remote。

## 构建策略

如果只修改文档、图标、弹窗样式或加载项前端，可复用已有 daemon 二进制。修改 Go 服务、安装脚本、运行时或模型时必须重新编译并重新打包。

正式发布必须同时交付：

    wps-read-aloud-comate_1.1.9_windows.exe
    wps-read-aloud-comate_1.1.9_amd64.deb
    wps-read-aloud-comate_1.1.9_arm64.deb
    cn.wps-read-aloud-comate_1.1.9_amd64.deb
    cn.wps-read-aloud-comate_1.1.9_arm64.deb
