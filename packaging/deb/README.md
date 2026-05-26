# Debian 安装包说明

本目录用于生成 Linux deb 安装包。正式发布由 packaging/build_all.py 统一调度。

## 输出

| CPU 架构 + 操作系统 | 包名 | 文件 |
| --- | --- | --- |
| x64 银河麒麟 V10 及以上 | wps-read-aloud-comate | dist/wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | wps-read-aloud-comate | dist/wps-read-aloud-comate_1.1.19_arm64.deb |
| x64 UOS V20 | cn.wps-read-aloud-comate | dist/cn.wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 UOS V20 | cn.wps-read-aloud-comate | dist/cn.wps-read-aloud-comate_1.1.19_arm64.deb |

## 安装内容

| CPU 架构 + 操作系统 | 程序目录 | 配置文件 | 文档目录 |
| --- | --- | --- | --- |
| x64 银河麒麟 V10 及以上 | /opt/wps-read-aloud-comate | /etc/wps-read-aloud-comate/config.yaml | /usr/share/doc/wps-read-aloud-comate |
| ARM64 银河麒麟 V10 及以上 | /opt/wps-read-aloud-comate | /etc/wps-read-aloud-comate/config.yaml | /usr/share/doc/wps-read-aloud-comate |
| x64 UOS V20 | /opt/apps/cn.wps-read-aloud-comate/files | /opt/apps/cn.wps-read-aloud-comate/files/config.yaml | /opt/apps/cn.wps-read-aloud-comate/files/doc |
| ARM64 UOS V20 | /opt/apps/cn.wps-read-aloud-comate/files | /opt/apps/cn.wps-read-aloud-comate/files/config.yaml | /opt/apps/cn.wps-read-aloud-comate/files/doc |

共同安装：

- /lib/systemd/system/wps-read-aloud-comate.service。
- /usr/bin/wps-read-aloud-comate-register。
- /var/log/wps-read-aloud-install.log。

## Linux 平台能力

- 仅支持 WPS Office 2019 for Linux 或更高版本。
- 本地服务固定监听 127.0.0.1:19860，不访问外网。
- 语音合成使用包内 Sherpa-onnx 与 vits-zh-hf-fanchen-C 模型。
- 文本预处理规则与 Windows 版本保持一致，包括中英文混读、数学符号、百分数、Office/WPS 固定读法和标点停顿。
- 播放层按当前桌面音频环境探测 pw-play、paplay 和 aplay。不同发行版的音频栈不同，因此播放器选择结果可能不同。
- Linux 版通常不显示 Windows 版 WPS 的原生第三方加载项确认框；是否弹出确认仍以目标机 WPS for Linux 的实际策略为准。

## 构建

列出目标：

    python packaging/build_all.py --list

构建全部目标：

    python packaging/build_all.py

构建单个 Linux 目标：

    python packaging/build_all.py --target kylin-amd64
    python packaging/build_all.py --target kylin-arm64
    python packaging/build_all.py --target uos-amd64
    python packaging/build_all.py --target uos-arm64

## 升级兼容

当前包名为 wps-read-aloud-comate。UOS 包名为 cn.wps-read-aloud-comate。新版本不强制移除旧包名，安装时会停用旧版 wps-tts.service，再启用新的 wps-read-aloud-comate.service，避免旧包维护脚本异常导致升级中断。同包名升级时会清理旧服务文件、旧注册脚本和旧版废弃语音引擎目录。当前版本只使用 Sherpa-onnx 与 vits-zh-hf-fanchen-C 模型。

