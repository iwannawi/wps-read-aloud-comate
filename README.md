# WPS 文档朗读助手

![WPS 文档朗读助手](docs/assets/readme-promo.png)

“WPS 文档朗读助手”是一套面向 WPS 文字的本地离线朗读加载项。加载项在 WPS 顶部新增“文档朗读”选项卡，提供开始朗读、停止朗读、朗读方式、朗读语速、状态检查和关于信息等功能。

## 适用环境

| 目标 | CPU 架构 + 操作系统 | WPS 要求 | 安装包 |
| --- | --- | --- | --- |
| Windows | x86/x64 Windows 10/11 | WPS Office 2019 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.4_windows.exe |
| 银河麒麟 | x64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.4_amd64.deb |
| 银河麒麟 | ARM64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.4_arm64.deb |
| UOS | x64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | cn.wps-read-aloud-comate_1.1.4_amd64.deb |
| UOS | ARM64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | cn.wps-read-aloud-comate_1.1.4_arm64.deb |

通用要求：

- 允许安装 WPS JS 加载项。
- WPS JS API 可读取 WPS 文字选区、光标位置和正文内容。
- 本机允许访问 127.0.0.1。
- 目标机无需联网下载语音引擎或模型。

## 技术方案

| 模块 | 作用 |
| --- | --- |
| WPS JS 加载项 | 提供顶部“文档朗读”选项卡，读取文档内容，控制按钮状态，同步选中当前朗读语句。 |
| Go 本地朗读服务 | 监听 127.0.0.1:19860，处理切句、预处理、合成调度、播放、状态检查和日志。 |
| Sherpa-onnx 离线 TTS | 使用 vits-zh-hf-fanchen-C 中文模型，安装包内置运行文件、模型和动态库。 |
| 系统音频播放 | Windows 使用 SoundPlayer；Linux 按环境探测可用播放器。 |

数据流：

    WPS 文字 -> 文档朗读选项卡 -> 本机朗读服务 -> Sherpa-onnx -> 系统播放器

服务只监听本机回环地址，不访问外网，不向局域网暴露端口。安装脚本只维护本项目自己的 WPS 加载项条目，尽量不影响其他加载项。

## 朗读能力

- 支持连页朗读和当页朗读。
- 有光标时从光标处开始；无可识别光标时按模式从文档或当前页开头开始。
- 当前朗读语句会在 WPS 文档中同步选中，并尽量保持可见。
- 默认语速为 1.2x，可选 0.75x、1x、1.2x、1.5x。
- 英文和数字会转换为逐字符中文读法，避免中英文混排时被跳过。
- 默认 1.2x 语速下，句内语义标点停顿约 400ms，句末停顿约 600ms；其他语速按比例调整。
- 启动朗读时按句动态预处理，累计约 100 字即可开始播放，减少句间等待。

## 项目结构

| 路径 | 内容 |
| --- | --- |
| addin | WPS JS 加载项源文件 |
| daemon | Go 本地朗读服务 |
| packaging | 多平台打包脚本 |
| resources/runtime | 按目标环境区分的原生运行时 |
| voices | 离线语音模型 |
| third_party_licenses | 第三方许可证和声明 |
| docs | 开发、打包和版本管理文档 |

## 构建

列出支持目标：

    python packaging/build_all.py --list

构建全部目标：

    python packaging/build_all.py

构建单个目标：

    python packaging/build_all.py --target windows
    python packaging/build_all.py --target kylin-amd64
    python packaging/build_all.py --target kylin-arm64
    python packaging/build_all.py --target uos-amd64
    python packaging/build_all.py --target uos-arm64

发布前检查：

    python packaging/verify_release_artifacts.py

检查内容包括安装包数量、校验文件、目标环境资源隔离、废弃引擎排除、内部经验文档排除和 CHECKSUMS.txt 一致性。

## 安装

| 目标 | 命令或操作 |
| --- | --- |
| x86/x64 Windows 10/11 | 运行 dist/wps-read-aloud-comate_1.1.4_windows.exe |
| x64 银河麒麟 V10 及以上 | sudo dpkg -i dist/wps-read-aloud-comate_1.1.4_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | sudo dpkg -i dist/wps-read-aloud-comate_1.1.4_arm64.deb |
| x64 UOS V20 | sudo dpkg -i dist/cn.wps-read-aloud-comate_1.1.4_amd64.deb |
| ARM64 UOS V20 | sudo dpkg -i dist/cn.wps-read-aloud-comate_1.1.4_arm64.deb |

Windows 安装程序会检测 WPS 安装路径、版本和可执行文件位数。加载项通过本地服务工作，不注入 WPS 进程，因此同一套 Windows 本地服务可服务 32 位和 64 位 WPS。

Linux 安装包会安装 systemd 服务、WPS 加载项注册脚本、说明文件、许可证文件和安装日志。安装后如 WPS 已打开，需要重启 WPS。

## 验证

Linux 服务检查：

    systemctl status wps-read-aloud-comate.service --no-pager
    curl http://127.0.0.1:19860/health
    curl http://127.0.0.1:19860/selftest

Windows 验证入口：

- 打开 WPS 文字。
- 确认顶部出现“文档朗读”选项卡。
- 点击“状态检查”，确认服务版本、语音引擎、自检结果和播放器状态正常。

## 版本管理

源码、脚本、配置、文档和许可证进入 Git。安装包、模型、离线引擎、工具链和构建缓存不进入普通 Git 提交。

详细规则见：

    docs/GIT_WORKFLOW.md
    docs/MULTI_PLATFORM_PACKAGING.md
