# 发布说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
Debian 包名：银河麒麟为 “wps-read-aloud-comate”，UOS 为 “cn.wps-read-aloud-comate”
版本：1.0.31
架构：x86/x64 Windows、Linux amd64、Linux arm64
适用操作系统：x86/x64 Windows、银河麒麟 x64/ARM64、UOS x64/ARM64，以及兼容 WPS JS 加载项和本地离线服务的同类系统
适用办公软件：Windows 平台要求 WPS Office 2019 或更高版本，推荐 WPS Office 最新稳定版；Linux 平台要求 WPS Office 2019 或更高版本，推荐最新版 WPS Office for Linux。
开发者：Zhang Jingyao
发布时间：20260519

## 本版本变更

- 修复 Windows 安装包无法识别已安装 WPS 个人版的问题。安装器会优先调用 64 位 Windows PowerShell，避免 32 位安装器落入 32 位注册表和文件系统视图后漏检 64 位 WPS。
- 扩展 Windows WPS 探测范围，新增读取 “App Paths\wps.exe”、Kingsoft/WPS 注册表键、开始菜单快捷方式、桌面快捷方式、“ProgramW6432”、Program Files、Program Files (x86)、LocalAppData 和 AppData 等入口。
- Windows 安装日志会记录实际检测到的 WPS 可执行文件路径、位数和版本，便于区分个人版、专业版、32 位或 64 位 WPS。
- Windows 安装器继续使用当前用户安装路径 “%LOCALAPPDATA%\Programs\WPS Read Aloud Comate”，避免普通用户写入系统 Program Files 目录时出现权限错误。
- 保持同一 Windows 安装包同时支持 32 位和 64 位 WPS。项目采用 WPS JS 加载项加独立本地朗读服务，不向 WPS 进程内注入 DLL，因此本地朗读服务位数不需要和 WPS 客户端位数一致。
- 更新第三方组件声明中的软件包名称、发布时间、适用系统和安装目录描述，避免残留旧包名或单平台表述。
- 精简发布说明，只保留当前版本信息和本版本实际变更，不再混入历史版本的累计修改记录。
- 修正已知限制中的音量描述，改为由目标操作系统、声卡设备或桌面环境声音设置统一控制，不再只描述某一个 Linux 发行版。

## 交付文件

    dist/wps-read-aloud-comate_1.0.31_windows.exe
    dist/wps-read-aloud-comate_1.0.31_amd64.deb
    dist/wps-read-aloud-comate_1.0.31_arm64.deb
    dist/cn.wps-read-aloud-comate_1.0.31_amd64.deb
    dist/cn.wps-read-aloud-comate_1.0.31_arm64.deb

## 安装提示

x86/x64 Windows 环境：

    运行 dist/wps-read-aloud-comate_1.0.31_windows.exe

银河麒麟 x64 环境：

    sudo dpkg -i dist/wps-read-aloud-comate_1.0.31_amd64.deb

银河麒麟 ARM64 环境：

    sudo dpkg -i dist/wps-read-aloud-comate_1.0.31_arm64.deb

UOS x64 环境：

    sudo dpkg -i dist/cn.wps-read-aloud-comate_1.0.31_amd64.deb

UOS ARM64 环境：

    sudo dpkg -i dist/cn.wps-read-aloud-comate_1.0.31_arm64.deb

如果 WPS 已经打开，请安装完成后重启 WPS，再使用顶部“文档朗读”选项卡。

## 已知限制

- 当前句选中和翻页依赖 WPS Office 对 Range、Selection、Page 等接口的支持；如果目标 WPS 版本接口行为不同，当页朗读会尽量回退到可读范围。
- 朗读过程中不能切换朗读方式和语速，需要停止朗读后再调整。
- 音量由目标操作系统、声卡设备或桌面环境声音设置控制，加载项内不提供音量调节。
