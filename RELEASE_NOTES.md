# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.3
发布时间：20260521
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

- Windows 安装器 logo 和任务栏图标改用透明背景 PNG 生成，适配图标规范。
- Windows 安装界面顶部改用 README 宣传图展示，提升中文文字清晰度，并将完成按钮居中显示。
- Windows 加载项注册改为本地加载项入口，避免 WPS 将加载项识别为 127.0.0.1 在线加载项。
- 朗读方式和朗读语速下拉项改用原生勾选状态，选项文字保持左端对齐。
- Linux 服务名改为 wps-read-aloud-comate.service，注册脚本改为 wps-read-aloud-comate-register。
- Linux 新包不再强制移除旧包名，安装时停用旧服务并启用新服务，避免旧包维护脚本异常导致升级中断。
- Linux 同包名升级时清理旧服务文件、旧注册脚本和废弃语音引擎目录，确保新版本完整接管。

## 修复

- 修复超长文档因前端字数上限提前中断，导致启动弹窗出现后朗读未执行的问题。
- 修复下拉项通过文字前缀显示勾选时视觉对齐不一致的问题。
- 修复全量构建时旧版本安装包残留导致发布目录校验失败的问题。
- 修复 Linux 同包名旧版本升级后可能残留旧服务或旧引擎文件的问题。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.3_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.3_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.3_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.3_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.3_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- 朗读过程中不能切换朗读方式和语速，需要停止朗读后再调整。
- 音量由目标操作系统、声卡设备或桌面环境声音设置控制，加载项内不提供音量调节。
