# 多平台安装包方案

本项目使用同一套源码，根据目标环境生成不同安装包。环境名称统一写成“CPU 架构 + 操作系统名”。

## 交付矩阵

| 目标 | 安装包 | 安装路径 | 启动方式 |
| --- | --- | --- | --- |
| x86/x64 Windows 10/11 | wps-read-aloud-comate_1.1.3_windows.exe | 用户选择的程序目录 | 当前用户登录自启动 |
| x64 银河麒麟 V10 及以上 | wps-read-aloud-comate_1.1.3_amd64.deb | /opt/wps-read-aloud-comate | systemd |
| ARM64 银河麒麟 V10 及以上 | wps-read-aloud-comate_1.1.3_arm64.deb | /opt/wps-read-aloud-comate | systemd |
| x64 UOS V20 | cn.wps-read-aloud-comate_1.1.3_amd64.deb | /opt/apps/cn.wps-read-aloud-comate/files | systemd |
| ARM64 UOS V20 | cn.wps-read-aloud-comate_1.1.3_arm64.deb | /opt/apps/cn.wps-read-aloud-comate/files | systemd |

## 共用内容

- addin：WPS JS 加载项。
- daemon：本地朗读服务源码。
- voices：离线语音模型。
- third_party_licenses：许可证和第三方声明。

## 差异内容

| 目标 | 差异点 |
| --- | --- |
| x86/x64 Windows 10/11 | Windows daemon、Windows Sherpa-onnx、安装器界面、当前用户自启动。 |
| x64 银河麒麟 V10 及以上 | x64 Linux daemon、x64 Linux Sherpa-onnx、Debian 控制脚本、systemd。 |
| ARM64 银河麒麟 V10 及以上 | ARM64 Linux daemon、ARM64 Linux Sherpa-onnx、Debian 控制脚本、systemd。 |
| x64 UOS V20 | x64 Linux daemon、x64 Linux Sherpa-onnx、UOS 应用目录、cn. 包名。 |
| ARM64 UOS V20 | ARM64 Linux daemon、ARM64 Linux Sherpa-onnx、UOS 应用目录、cn. 包名。 |

Windows 加载项通过 127.0.0.1 调用独立服务，不向 WPS 进程注入 DLL。同一套 Windows 服务可服务 32 位和 64 位 WPS，安装日志仍会记录 WPS 位数。

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
