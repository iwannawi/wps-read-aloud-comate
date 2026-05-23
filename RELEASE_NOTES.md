# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.9
发布时间：20260523
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

- 朗读片段生成优先使用 WPS 段落 Range 的真实 Start 和 End，不再只依赖整篇文本偏移推算。
- 每个朗读片段记录所属段落或单元格范围，后续选中校正只在所属范围内执行。
- Windows 安装界面改为窗体尺寸匹配内容，不再通过窗体滚动条展示安装信息。
- Windows 安装界面顶部宣传图按原始比例完整铺满窗体宽度。
- Windows 安装界面设置专属窗口图标，减少显示 PowerShell 默认图标的情况。

## 修复

- 修复目录项如 2.1 等短文本朗读正确但 WPS 选区可能落到其他目录项的问题。
- 修复表格内重复文字朗读正确但选区可能落到其他单元格的问题。
- 修复 Windows 安装界面因宣传图高度限制导致左右出现白边的问题。
- 修复 Windows 安装界面在高 DPI 环境下仍可能出现内容显示不完整的问题。

## 交付文件

| 目标 | 文件 |
| --- | --- |
| x86/x64 Windows 10/11 | dist/wps-read-aloud-comate_1.1.9_windows.exe |
| x64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.9_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | dist/wps-read-aloud-comate_1.1.9_arm64.deb |
| x64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.9_amd64.deb |
| ARM64 UOS V20 | dist/cn.wps-read-aloud-comate_1.1.9_arm64.deb |

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对文档 Range、Selection、Page 等接口的支持。
- 朗读过程中不能切换朗读方式和语速，需要停止朗读后再调整。
- 音量由目标操作系统、声卡设备或桌面环境声音设置控制，加载项内不提供音量调节。
