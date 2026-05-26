# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.19
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

- Windows 安装阶段改为直接启动本机朗读服务程序，再等待 127.0.0.1:19860 健康检查通过；开机自启动仍使用 start-daemon.ps1 启动脚本。
- Windows 覆盖安装前会清理安装目录中的本项目旧载荷、旧说明文件和旧压缩包残留，避免旧版本文件混入新版本。
- Windows 安装完成后继续使用 publish.xml 在线发布模式，并保留当前用户 Run 自启动项，确保 WPS 启动时加载项地址可访问。
- Windows 开始菜单卸载入口优先写入当前用户开始菜单；在管理员上下文中再额外写入公共开始菜单，避免非管理员安装日志出现公共目录权限报错。

## 修复

- 修复 Windows 安装日志显示“本机朗读服务启动失败”的问题。原因是安装器通过隐藏 PowerShell 启动脚本间接拉起服务时，在部分 Windows 11 环境下未能稳定完成启动；新版本改为安装器直接启动 daemon。
- 修复升级安装后安装目录可能残留上一版本文件的问题。
- 优化安装失败定位：服务程序、配置文件、发布地址和健康检查在安装阶段直接验证。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.19_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.19_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.19_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- Windows 版 WPS 是否显示首次安全确认框由 WPS 客户端策略决定；本项目不伪造或关闭 WPS 原生安全确认。

