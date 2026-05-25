# 多平台安装包方案

本项目使用同一套源码，根据目标环境生成不同安装包。环境名称统一写成“CPU 架构 + 操作系统名”。

交付时必须同时说明共通能力和平台差异。Windows、银河麒麟和 UOS 共用 WPS JS 加载项、本地 Go 服务、Sherpa-onnx 模型和文本预处理规则；安装方式、启动方式、播放层、日志位置和 WPS 首次许可提示存在差异。

## 交付矩阵

| 目标 | 安装包 | 安装路径 | 启动方式 |
| --- | --- | --- | --- |
| x86/x64 Windows 10/11 | wps-read-aloud-comate_1.1.18_windows.exe | 用户选择的程序目录 | publish.xml 注册加载项；本机服务安装后启动并随当前用户登录自启动 |
| x64 银河麒麟 V10 及以上 | wps-read-aloud-comate_1.1.18_amd64.deb | /opt/wps-read-aloud-comate | systemd |
| ARM64 银河麒麟 V10 及以上 | wps-read-aloud-comate_1.1.18_arm64.deb | /opt/wps-read-aloud-comate | systemd |
| x64 UOS V20 | cn.wps-read-aloud-comate_1.1.18_amd64.deb | /opt/apps/cn.wps-read-aloud-comate/files | systemd |
| ARM64 UOS V20 | cn.wps-read-aloud-comate_1.1.18_arm64.deb | /opt/apps/cn.wps-read-aloud-comate/files | systemd |

## 共用内容

- addin：WPS JS 加载项。
- daemon：本地朗读服务源码。
- voices：离线语音模型。
- third_party_licenses：许可证和第三方声明。
- 文本预处理规则：中英文混读、数学符号、百分数、Office/WPS 固定读法和标点停顿规则保持跨平台一致。

## 差异内容

| 目标 | 差异点 |
| --- | --- |
| x86/x64 Windows 10/11 | Windows daemon、Windows Sherpa-onnx、图形安装器、publish.xml 加载项注册、Run 自启动、WinMM 播放、开始菜单和控制面板卸载入口、WPS 原生第三方加载项许可确认。 |
| x64 银河麒麟 V10 及以上 | x64 Linux daemon、x64 Linux Sherpa-onnx、Debian 控制脚本、systemd、Linux 桌面音频播放器探测。 |
| ARM64 银河麒麟 V10 及以上 | ARM64 Linux daemon、ARM64 Linux Sherpa-onnx、Debian 控制脚本、systemd、Linux 桌面音频播放器探测。 |
| x64 UOS V20 | x64 Linux daemon、x64 Linux Sherpa-onnx、UOS 应用目录、cn. 包名、systemd、Linux 桌面音频播放器探测。 |
| ARM64 UOS V20 | ARM64 Linux daemon、ARM64 Linux Sherpa-onnx、UOS 应用目录、cn. 包名、systemd、Linux 桌面音频播放器探测。 |

Windows 加载项通过 127.0.0.1 调用独立服务，不向 WPS 进程注入 DLL。同一套 Windows 服务可服务 32 位和 64 位 WPS，安装日志仍会记录 WPS 位数。Windows 顶部选项卡使用 WPS 官方 publish.xml 模式：安装器在当前用户 jsaddons 下写入 jspluginonline 入口，地址指向 http://127.0.0.1:19860/addin/；本机服务安装后立即启动，并通过当前用户 Run 自启动项在登录后自动启动。语音合成由 sherpa-onnx-offline-tts.exe 子进程完成，只在朗读、自检等需要合成音频时启动，停止朗读会终止当前合成和播放，但不会关闭本机发布服务。Windows WPS 可能显示原生第三方加载项许可确认框，该确认框由 WPS 客户端安全策略生成，安装包只能保留已允许记录，不能合规绕过。

Windows 安装包必须注册开始菜单卸载入口和当前用户“应用和功能”卸载信息。开始菜单入口同时写入当前用户 Programs 和公共 CommonPrograms 下的“WPS文档朗读助手”文件夹。卸载时必须清理安装目录、WPS 加载项注册、授权缓存、本项目开始菜单入口、卸载注册表、Run 自启动项、旧计划任务和本项目写入过的 OEM 残留指向项。

Linux 安装包使用 systemd 管理服务，音频播放依赖当前桌面环境可用的 pw-play、paplay 或 aplay。银河麒麟包和 UOS 包的安装路径与包名不同，不能混用打包规范。

## 构建命令

列出目标：

    python packaging/build_all.py --list

构建全部目标：

    python packaging/build_all.py

构建单个目标：

    python packaging/build_all.py --target windows
    python packaging/build_all.py --target kylin-amd64
    python packaging/build_all.py --target kylin-arm64
    python packaging/build_all.py --target uos-amd64
    python packaging/build_all.py --target uos-arm64

## 发布前检查

    python packaging/verify_release_artifacts.py

检查项：

- 五个安装包全部存在。
- 五个 SHA256 文件全部存在。
- CHECKSUMS.txt 与安装包一致。
- 每个安装包只包含本目标需要的二进制和服务文件。
- 不包含 Piper、eSpeak NG 等废弃资源。
- 不包含内部经验文档。

任何目标缺失运行时、模型、daemon 或校验文件，都不得创建 Release。

