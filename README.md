# WPS 文档朗读助手

目标环境：
- x86/x64 Windows 环境
- x64 / ARM64 银河麒麟操作系统
- x64 / ARM64 UOS 操作系统
- 以及兼容 WPS JS 加载项和本地离线服务的同类系统
- Windows 平台：WPS Office 2019 或更高版本，推荐 WPS Office 最新稳定版
- Linux 平台：WPS Office 2019 或更高版本，推荐最新版 WPS Office for Linux
- 允许安装 WPS JS 加载项
- WPS JS API 可读取 WPS 文字选区和全文
- 本机允许访问 “127.0.0.1”

本项目提供“WPS 文档朗读助手”，是一套面向 WPS 文字的离线文档朗读方案。软件在 WPS 顶部新增“文档朗读”选项卡，不使用独立右侧面板；用户可以直接通过 Ribbon 按钮启动朗读、停止朗读、选择朗读方式、选择朗读语速、检查服务状态和查看关于信息。

整体方案由三部分组成：
- WPS JS 加载项：负责读取 WPS 文字中的光标位置、当前页、选区和正文内容，控制顶部选项卡按钮状态，并在朗读过程中同步选中当前朗读语句，尽量保持当前语句在文档视图中可见。
- Go 本地朗读服务：只监听 “127.0.0.1:19860”，负责文本切句、预处理、语音合成调度、音频播放、状态检查、日志和异常处理。
- 离线语音引擎与模型：使用 Sherpa-onnx 离线 TTS，当前主模型为 “vits-zh-hf-fanchen-C”。安装包内置运行所需的模型、引擎、动态库、说明文件和许可证文件，目标机不需要联网下载依赖。

朗读功能以中文文档为主，兼顾中英文数字混读。中文按句朗读；英文和数字会在文本预处理阶段转换为逐字符中文读法，避免模型跳过英文或数字。默认 “1.2x” 语速下，句内语义标点按约 “400ms” 节奏停顿，句末追加约 “600ms” 静音；其他语速下停顿时长随语速等比例缩放。

当前项目采用同一套源码、多平台打包的交付方式。每次正式出版本都生成五类安装包：x86/x64 Windows exe 安装程序、银河麒麟 amd64 deb、银河麒麟 arm64 deb、UOS amd64 deb、UOS arm64 deb。各平台共用加载项前端、服务端业务逻辑、文档和模型资源；差异部分集中在原生语音引擎、daemon 二进制、服务启动方式、包名和安装路径。

## 项目结构

    addin/                         WPS JS 加载项源文件
    daemon/                        Go 本地服务，监听 127.0.0.1:19860
    packaging/deb/                 .deb 打包脚本和 Debian 控制脚本
    packaging/windows/             Windows 安装包脚本
    packaging/platforms.json       多平台安装包矩阵
    packaging/sync_addin_web.py    同步 addin/ 到 Go embed 目录
    resources/runtime/             按系统和架构区分的原生语音引擎资源
    third_party_licenses/          第三方许可证和声明
    engines/                       打包时放置 Sherpa-onnx 运行文件
    voices/                        打包时放置离线语音模型

## 工作方式

    WPS 文字 -> 文档朗读选项卡 -> http://127.0.0.1:19860 -> Go 服务 -> Sherpa-onnx VITS fanchen-C -> 系统播放器

服务只监听 “127.0.0.1:19860”，不访问外网。加载项与服务端通过本机回环地址通信，避免暴露到局域网。安装脚本会尽量只增删本项目自己的 WPS 加载项条目，不覆盖其他已安装加载项；Linux 包也包含旧包名的冲突和替换声明，降低升级时的文件归属冲突风险。

朗读时按完整语句切分、逐句合成并播放；加载项会在 WPS 文档中同步选中当前朗读语句，并尽量保持当前语句可见。启动朗读时，服务按句动态预处理，累计文本达到约 100 字即可开始播放；如果第一句已超过 100 字，只等待第一句合成完成，减少启动等待。

## 准备离线依赖

打包前需要准备：

    resources/runtime/windows-x86/sherpa-onnx/
    resources/runtime/linux-amd64/sherpa-onnx/
    resources/runtime/linux-arm64/sherpa-onnx/
    voices/sherpa/vits-zh-hf-fanchen-C/

这些文件会被打入对应安装包。银河麒麟包安装到 “/opt/wps-read-aloud-comate”；UOS 包按 UOS 应用目录规范安装到 “/opt/apps/cn.wps-read-aloud-comate/files”；Windows 系统安装到用户选择的程序目录。正式构建统一从 “resources/runtime” 读取平台运行时，不再从旧版 “engines” 目录复制，避免把目标环境不需要的库和废弃语音引擎带入安装包。

多平台安装包说明见：

    docs/MULTI_PLATFORM_PACKAGING.md

## 构建

列出所有支持的安装包目标：

    python3 packaging/build_all.py --list

构建全部目标：

    python3 packaging/build_all.py

按需构建单个目标时，使用 “--list” 看到的目标编号，例如：

    python3 packaging/build_all.py --target windows
    python3 packaging/build_all.py --target kylin-amd64
    python3 packaging/build_all.py --target kylin-arm64
    python3 packaging/build_all.py --target uos-amd64
    python3 packaging/build_all.py --target uos-arm64

如果某个目标缺少对应系统和架构的 Sherpa-onnx 运行文件、模型资源或 daemon 二进制，脚本会停止并提示缺失路径，避免生成不可用安装包。

正式发布版本必须一次性生成五类安装包，并通过发布前检查：

    python packaging/verify_release_artifacts.py

该检查会确认五个安装包、五个 SHA256 文件和 “CHECKSUMS.txt” 完全一致，同时检查安装包内没有混入目标环境不需要的资源，例如 Windows 包不包含 Linux systemd 文件，Linux 包不包含 Windows exe，所有包都不包含 Piper 或 eSpeak NG 等已弃用资源。

当前版本的五类交付文件：

    dist/wps-read-aloud-comate_1.0.32_windows.exe
    dist/wps-read-aloud-comate_1.0.32_amd64.deb
    dist/wps-read-aloud-comate_1.0.32_arm64.deb
    dist/cn.wps-read-aloud-comate_1.0.32_amd64.deb
    dist/cn.wps-read-aloud-comate_1.0.32_arm64.deb

## 安装

x86/x64 Windows 环境：

    运行 dist/wps-read-aloud-comate_1.0.32_windows.exe

Windows 安装程序会先检测本机 WPS Office 的安装路径、版本和可执行文件位数。由于本项目采用 WPS JS 加载项加独立本地朗读服务的架构，不向 WPS 进程内注入 DLL，因此同一个 Windows 本地朗读服务可以同时服务 32 位和 64 位 WPS；检测位数主要用于安装日志和问题排查。

银河麒麟 x64 环境：

    sudo dpkg -i dist/wps-read-aloud-comate_1.0.32_amd64.deb

银河麒麟 ARM64 环境：

    sudo dpkg -i dist/wps-read-aloud-comate_1.0.32_arm64.deb

UOS x64 环境：

    sudo dpkg -i dist/cn.wps-read-aloud-comate_1.0.32_amd64.deb

UOS ARM64 环境：

    sudo dpkg -i dist/cn.wps-read-aloud-comate_1.0.32_arm64.deb

Linux 安装包会：
- 银河麒麟：安装程序文件到 “/opt/wps-read-aloud-comate”，配置文件到 “/etc/wps-read-aloud-comate/config.yaml”
- UOS：安装程序文件和配置文件到 “/opt/apps/cn.wps-read-aloud-comate/files”
- 安装并启动 “wps-tts.service”
- 为已有普通用户注册 WPS JS 加载项
- 写入安装日志 “/var/log/wps-read-aloud-install.log”
- 安装第三方组件许可证和交付说明；银河麒麟路径为 “/usr/share/doc/wps-read-aloud-comate”，UOS 路径为 “/opt/apps/cn.wps-read-aloud-comate/files/doc”

如果 WPS 已打开，安装后需要重启 WPS 才能加载新版“文档朗读”选项卡。

## 验证

    systemctl status wps-tts.service --no-pager
    curl http://127.0.0.1:19860/health
    curl http://127.0.0.1:19860/selftest

打开 WPS 文字后，顶部应出现“文档朗读”选项卡。优先点击“状态检查”，如果弹出服务状态提示，说明 Ribbon 按钮回调已正常触发。

## 版本管理

源码、脚本、配置、文档和许可证进入 Git。以下内容不进入普通 Git：
- “dist/”
- “engines/”
- “tools/”
- “voices/sherpa/”
- 构建缓存和下载缓存

详细版本管理规则见：

    docs/GIT_WORKFLOW.md
    docs/CODEX_AUTOMATION.md
