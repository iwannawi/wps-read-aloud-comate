# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.17
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

- Windows 离线加载项注册补齐真实 7z 包生成流程。安装器会在写入本地“文档朗读助手_版本号”目录后，使用 Windows 10/11 自带 tar.exe 生成同名 .7z 离线包，满足 WPS 离线 jsplugin 对压缩包地址的校验要求。
- Windows 安装校验增加离线包格式检查，确保生成文件为真实 7z 格式。

## 修复

- 修复 Windows 端仅预置本地加载项目录、未生成真实离线包，导致部分 WPS 版本启动后看不到“文档朗读”选项卡的问题。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.17_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.17_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.17_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.17_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.17_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- Windows 版 WPS 是否显示首次安全确认框由 WPS 客户端策略决定；本项目不伪造或关闭 WPS 原生安全确认。

