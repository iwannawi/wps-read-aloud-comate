# Debian 安装包说明

本目录用于生成银河麒麟和 UOS 的 Debian 安装包。正式出版本时由上级脚本 “packaging/build_all.py” 统一构建，不再保留单独面向某一系统或某一架构的安装脚本。

## 输出文件

当前版本的 Linux 输出文件为：

    dist/wps-read-aloud-comate_1.0.31_amd64.deb
    dist/wps-read-aloud-comate_1.0.31_arm64.deb
    dist/cn.wps-read-aloud-comate_1.0.31_amd64.deb
    dist/cn.wps-read-aloud-comate_1.0.31_arm64.deb

银河麒麟包的内部包名为 “wps-read-aloud-comate”。UOS 包按照 UOS 应用包名习惯使用 “cn.wps-read-aloud-comate”。文件名统一采用 “包名_版本_架构.deb”。

## 安装内容

银河麒麟包：

- 程序目录：“/opt/wps-read-aloud-comate”
- 配置文件：“/etc/wps-read-aloud-comate/config.yaml”
- 说明和许可证：“/usr/share/doc/wps-read-aloud-comate”

UOS 包：

- 程序目录：“/opt/apps/cn.wps-read-aloud-comate/files”
- 配置文件：“/opt/apps/cn.wps-read-aloud-comate/files/config.yaml”
- 说明和许可证：“/opt/apps/cn.wps-read-aloud-comate/files/doc”

两个 Linux 包共同安装：

- 系统服务：“/lib/systemd/system/wps-tts.service”
- WPS 加载项注册脚本：“/usr/bin/wps-read-aloud-register”
- 安装日志：“/var/log/wps-read-aloud-install.log”

## 构建命令

列出全部目标：

    python3 packaging/build_all.py --list

构建全部目标：

    python3 packaging/build_all.py

按需构建单个 Linux 目标时，使用 “--list” 输出的目标编号：

    python3 packaging/build_all.py --target kylin-amd64
    python3 packaging/build_all.py --target kylin-arm64
    python3 packaging/build_all.py --target uos-amd64
    python3 packaging/build_all.py --target uos-arm64

## 升级兼容

当前包名从旧版 “wps-read-aloud-zhangjingyao” 调整为 “wps-read-aloud-comate”。UOS 包和银河麒麟包之间也互相声明冲突和替换，避免同一台机器同时安装两个会注册同名 WPS 加载项、同名 systemd 服务和同一端口服务的包。

安装脚本同时识别旧版和新版所有权标记，避免从旧版本升级时误判项目目录为外部目录。
