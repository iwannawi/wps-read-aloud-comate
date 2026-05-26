# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.20
发布时间：20260526
开发者：Zhang Jingyao

## 适用环境

| CPU 架构 + 操作系统 | WPS 要求 |
| --- | --- |
| x86/x64 Windows 10/11 | WPS Office 2019 或更高版本，推荐最新稳定版 |
| x64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 |
| ARM64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 |
| x64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 |
| ARM64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 |

## 变更

- Windows 开始菜单改为创建“WPS文档朗读助手”文件夹，文件夹内包含“打开安装目录”和“卸载 WPS文档朗读助手”两个快捷方式，避免 Windows 11 将单快捷方式文件夹扁平化显示。
- Linux 卸载流程补充用户 WPS 加载项注册清理：remove 时清理普通用户主目录下的加载项副本、publish.xml 和 jsplugins.xml 中的本项目条目；purge 时继续清理配置目录和运行数据。
- README 增加 Windows、银河麒麟、UOS 的卸载方式和清理范围说明。
- README 精简朗读能力描述，不再展开固定词汇的具体读法。

## 修复

- 修复 Windows 11 开始菜单中可能直接显示“卸载 WPS文档朗读助手”，而不是显示一级“WPS文档朗读助手”文件夹的问题。
- 修复 Linux 卸载后用户目录中可能残留 WPS 加载项注册项的问题。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.20_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.20_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.20_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.20_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.20_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- Windows 版 WPS 是否显示首次安全确认框由 WPS 客户端策略决定；本项目不伪造或关闭 WPS 原生安全确认。

