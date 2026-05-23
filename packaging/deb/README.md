# Debian 安装包说明

本目录用于生成 Linux deb 安装包。正式发布由 packaging/build_all.py 统一调度。

## 输出

| CPU 架构 + 操作系统 | 包名 | 文件 |
| --- | --- | --- |
| x64 银河麒麟 V10 及以上 | wps-read-aloud-comate | dist/wps-read-aloud-comate_1.1.9_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | wps-read-aloud-comate | dist/wps-read-aloud-comate_1.1.9_arm64.deb |
| x64 UOS V20 | cn.wps-read-aloud-comate | dist/cn.wps-read-aloud-comate_1.1.9_amd64.deb |
| ARM64 UOS V20 | cn.wps-read-aloud-comate | dist/cn.wps-read-aloud-comate_1.1.9_arm64.deb |

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

当前包名为 wps-read-aloud-comate。UOS 包名为 cn.wps-read-aloud-comate。新版本不强制移除旧包名，安装时会停用旧版 wps-tts.service，再启用新的 wps-read-aloud-comate.service，避免旧包维护脚本异常导致升级中断。同包名升级时会清理旧服务文件、旧注册脚本和已废弃的 piper/espeak 目录。
