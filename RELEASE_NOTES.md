# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.18
发布时间：20260525
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

- Windows 端恢复为已验证可显示选项卡的 publish 模式：写入 publish.xml 的 jspluginonline 入口，地址为 http://127.0.0.1:19860/addin/。
- Windows 安装后立即启动本机朗读服务，并写入当前用户 Run 自启动项，确保 WPS 启动时加载项发布地址可访问。
- Windows 本机朗读服务取消空闲自动退出，保持常驻，直到用户卸载、系统退出或进程被手动结束。
- Windows 开始菜单卸载入口同时写入当前用户 Programs 和公共 CommonPrograms 下的“WPS文档朗读助手”文件夹。

## 修复

- 修复 Windows 端 OEM 离线模式下部分 WPS 版本仍看不到“文档朗读”选项卡的问题。
- 修复 Windows 11 开始菜单中“WPS文档朗读助手”文件夹可能未显示的问题。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.18_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.18_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.18_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.18_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.18_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- Windows 版 WPS 是否显示首次安全确认框由 WPS 客户端策略决定；本项目不伪造或关闭 WPS 原生安全确认。

