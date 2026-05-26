# WPS 文档朗读助手

![WPS 文档朗读助手](docs/assets/readme-promo.png)

“WPS 文档朗读助手”是一套面向 WPS 文字的本地离线朗读加载项。安装后，WPS 顶部会新增“文档朗读”选项卡，提供开始朗读、停止朗读、朗读方式、朗读语速、状态检查和关于信息等功能。

软件采用同一套源码、多平台分包交付方式。每个安装包只包含本目标环境需要的本地服务、离线语音引擎、语音模型和运行库，不依赖联网下载。

## 适用环境

| 目标 | CPU 架构 + 操作系统 | WPS 要求 | 安装包 |
| --- | --- | --- | --- |
| Windows | x86/x64 Windows 10/11 | WPS Office 2019 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.20_windows.exe |
| 银河麒麟 | x64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.20_amd64.deb |
| 银河麒麟 | ARM64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | wps-read-aloud-comate_1.1.20_arm64.deb |
| UOS | x64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | cn.wps-read-aloud-comate_1.1.20_amd64.deb |
| UOS | ARM64 UOS V20 | WPS Office 2019 for Linux 或更高版本，推荐最新稳定版 | cn.wps-read-aloud-comate_1.1.20_arm64.deb |

通用要求：

- 允许安装 WPS JS 加载项。
- WPS JS API 可读取 WPS 文字选区、光标位置和正文内容。
- 本机允许访问 127.0.0.1。
- 目标机无需联网下载语音引擎或模型。

## 技术方案

### 共通架构

| 模块 | 当前实现 |
| --- | --- |
| WPS JS 加载项 | 提供“文档朗读”选项卡，读取光标、选区、页码和正文 Range，按句生成朗读任务，并在朗读时同步选中当前语句。 |
| Go 本地朗读服务 | 固定监听 127.0.0.1:19860，只接受本机访问，负责请求校验、文本预处理、语音合成调度、预合成缓存、播放控制、状态检查和日志输出。 |
| Sherpa-onnx 离线 TTS | 使用内置 vits-zh-hf-fanchen-C 中文 VITS 模型，不再包含 Piper、eSpeak NG 或双模型切换方案。 |

数据流：

    WPS 文字 -> 文档朗读选项卡 -> 本机朗读服务 -> Sherpa-onnx -> 平台播放层

服务只监听本机回环地址，不访问外网，不向局域网暴露端口。安装脚本只维护本项目自己的 WPS 加载项条目，尽量不影响其他加载项。

### 平台差异

| 项目 | x86/x64 Windows 10/11 | x64/ARM64 银河麒麟 V10 及以上 | x64/ARM64 UOS V20 |
| --- | --- | --- | --- |
| 安装包 | exe 图形安装程序 | deb 安装包 | cn. 开头的 deb 安装包 |
| 安装目录 | 当前用户目录，默认 %LOCALAPPDATA%\Programs\WPS Read Aloud Comate | /opt/wps-read-aloud-comate | /opt/apps/cn.wps-read-aloud-comate/files |
| 服务启动 | 加载项采用 publish.xml 在线发布模式；本地服务安装后立即启动，并随当前用户登录自动启动 | systemd 服务 wps-read-aloud-comate.service | systemd 服务 wps-read-aloud-comate.service |
| 音频播放 | Windows 原生 WinMM 播放 WAV，停止时调用 WinMM 中断当前声音 | 按当前桌面音频环境探测 pw-play、paplay、aplay | 按当前桌面音频环境探测 pw-play、paplay、aplay |
| WPS 加载项注册 | 写入当前用户 jsaddons 下的 publish.xml，地址指向 http://127.0.0.1:19860/addin/ | 注册到用户主目录下的 WPS jsaddons 配置 | 注册到用户主目录下的 WPS jsaddons 配置 |
| 首次许可弹窗 | Windows WPS 可能显示原生第三方加载项许可确认框，项目只能保留已允许记录，不能合规绕过 | 通常不显示 Windows 同款确认框，具体取决于 WPS for Linux 策略 | 通常不显示 Windows 同款确认框，具体取决于 WPS for Linux 策略 |
| 日志 | %LOCALAPPDATA%\WPSReadAloudComate\Logs\install.log，服务日志随安装目录和进程输出管理 | /var/log/wps-read-aloud-install.log，服务日志通过 journalctl 查看 | /var/log/wps-read-aloud-install.log，服务日志通过 journalctl 查看 |

## 朗读能力

- 支持“连页朗读”和“当页朗读”。有光标时从光标处开始；没有可识别光标时，连页朗读从文档开头开始，当页朗读从当前页开头开始。
- 朗读过程中按完整语句同步选中 WPS 文档中的对应 Range，并尽量保持当前语句所在页可见。
- 支持 0.75x、1x、1.2x、1.5x 四档语速，默认 1.2x。朗读进行中不能切换朗读方式和语速，需要停止后调整。
- 普通英文和数字会转换为逐字符中文读法，避免中英文混排时被跳过。
- 部分中英文混排词汇会做固定读法处理，避免被跳过或读成不可理解的连读。
- 常见数学内容会按阅读习惯预处理，例如“10%”读作“百分之十”，“+、-、×、÷、≥、±、√”等符号读作对应中文名称。
- 默认 1.2x 语速下，逗号、顿号、冒号、分号等句内语义标点保留约 400ms 停顿，句末追加约 600ms 停顿；其他语速按比例调整。
- 启动朗读时按句动态预合成。服务按累计文本长度和句数上限控制预合成窗口，减少短句之间的等待，也避免长文档一次性启动大量合成任务。
- 表格空单元格、图片、嵌入对象和其他非文本元素会跳过，不朗读“不朗读对象”或“空白内容”。
- Windows 端停止朗读会调用 WinMM 停止当前 WAV，尽量不等待当前句自然结束；Linux 端会终止当前播放进程组。

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
| x86/x64 Windows 10/11 | 运行 dist/wps-read-aloud-comate_1.1.20_windows.exe |
| x64 银河麒麟 V10 及以上 | sudo dpkg -i dist/wps-read-aloud-comate_1.1.20_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | sudo dpkg -i dist/wps-read-aloud-comate_1.1.20_arm64.deb |
| x64 UOS V20 | sudo dpkg -i dist/cn.wps-read-aloud-comate_1.1.20_amd64.deb |
| ARM64 UOS V20 | sudo dpkg -i dist/cn.wps-read-aloud-comate_1.1.20_arm64.deb |

Windows 安装程序会检测 WPS 安装路径、版本和可执行文件位数。加载项通过本地服务工作，不注入 WPS 进程，因此同一套 Windows 本地服务可服务 32 位和 64 位 WPS。Windows 端默认安装到当前用户目录，写入当前用户 WPS jsaddons 配置、当前用户 Run 自启动项和当前用户卸载注册表；安装器只清理本项目历史版本写入过的旧 OEM 指向项，不再依赖 OEM 离线模式。

Windows 版本采用 publish.xml 在线发布模式。安装时会写入当前用户 jsaddons 下的 publish.xml，加载项地址为 http://127.0.0.1:19860/addin/；安装器会立即启动本机朗读服务，并在当前用户 Run 自启动项中注册服务启动脚本，确保 WPS 打开时加载项地址可访问。安装完成后请彻底退出并重新打开 WPS。停止朗读只停止当前朗读会话、播放和正在运行的语音合成子进程，不关闭本机发布服务。卸载入口会写入当前用户开始菜单“WPS文档朗读助手”文件夹和系统“应用和功能”；如安装器在管理员上下文运行，会额外写入公共开始菜单入口。卸载时清理安装文件、WPS 加载项配置、授权缓存、Run 自启动项、旧计划任务、开始菜单入口、注册表项和本项目写入过的 OEM 残留指向项。

Windows WPS 首次信任第三方加载项时，可能显示 WPS 原生安全确认框。该弹窗由 Windows 版 WPS 客户端安全策略生成，项目不能合规地绕过或伪造关闭。安装脚本会保留已允许记录，升级安装时不主动清除授权缓存，尽量避免重复出现。

Linux 安装包会安装 systemd 服务、WPS 加载项注册脚本、说明文件、许可证文件和安装日志。银河麒麟包安装到 /opt/wps-read-aloud-comate，UOS 包安装到 /opt/apps/cn.wps-read-aloud-comate/files。安装后如 WPS 已打开，需要彻底退出并重新打开 WPS。

## 卸载

| 目标 | 操作 | 清理范围 |
| --- | --- | --- |
| x86/x64 Windows 10/11 | 开始菜单“WPS文档朗读助手”文件夹中运行“卸载 WPS文档朗读助手”，或在系统“应用和功能”中卸载 | 停止本机服务，清理安装目录、WPS 加载项配置、授权缓存、Run 自启动项、旧计划任务、开始菜单入口、卸载注册表和本项目旧 OEM 指向项 |
| x64/ARM64 银河麒麟 V10 及以上 | sudo apt remove wps-read-aloud-comate；如需彻底清理配置，执行 sudo apt purge wps-read-aloud-comate | remove 会停止 systemd 服务、移除包内文件并清理用户 WPS 加载项注册；purge 会额外清理配置目录和运行数据 |
| x64/ARM64 UOS V20 | sudo apt remove cn.wps-read-aloud-comate；如需彻底清理配置，执行 sudo apt purge cn.wps-read-aloud-comate | remove 会停止 systemd 服务、移除包内文件并清理用户 WPS 加载项注册；purge 会额外清理配置目录和运行数据 |

## 验证

Linux 服务检查：

    systemctl status wps-read-aloud-comate.service --no-pager
    curl http://127.0.0.1:19860/health
    curl http://127.0.0.1:19860/selftest

Windows 验证入口：

- 打开 WPS 文字。
- 确认顶部出现“文档朗读”选项卡。
- 点击“状态检查”，确认服务版本、语音引擎、自检结果和播放器状态正常。
- 首次出现 WPS 原生加载项许可确认框时，确认名称为“文档朗读助手”，来源为本机 127.0.0.1 服务。点击“允许”后，升级安装不应主动清除该允许记录。

## 版本管理

源码、脚本、配置、文档和许可证进入 Git。安装包、模型、离线引擎、工具链和构建缓存不进入普通 Git 提交。

详细规则见：

    docs/GIT_WORKFLOW.md
    docs/MULTI_PLATFORM_PACKAGING.md

