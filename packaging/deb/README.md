# Debian 安装包说明

本目录用于生成 ARM64 麒麟操作系统可安装的企业交付包。

## 输出文件

```text
dist/wps-read-aloud-xc_1.0.24_arm64.deb
```

Debian 内部包名为 `wps-read-aloud-xc`。安装包文件名统一使用小写，便于 Linux 环境和脚本稳定处理。

## 安装内容

- `/opt/wps-read-aloud/`：加载项、Go 服务、Sherpa-onnx 引擎和语音模型。
- `/etc/wps-read-aloud/config.yaml`：本地服务配置。
- `/lib/systemd/system/wps-tts.service`：系统服务。
- `/usr/bin/wps-read-aloud-register`：WPS 加载项注册脚本。
- `/usr/share/doc/wps-read-aloud-xc/`：发布说明、验收测试、第三方许可证和源码获取说明。
- `/var/log/wps-read-aloud-install.log`：安装日志。

## 构建命令

```bash
python3 packaging/deb/build_deb.py
```

兼容脚本 `build-deb.sh` 和 `build-deb.ps1` 也会转调同一个 Python 打包入口。

## 升级兼容

当前包名从旧版 `wps-read-aloud-zhangjingyao` 调整为 `wps-read-aloud-xc`。控制文件包含：

```text
Conflicts: wps-read-aloud-zhangjingyao
Replaces: wps-read-aloud-zhangjingyao
```

安装脚本同时识别旧版和新版所有权标记，避免从旧版本升级时误判 `/opt/wps-read-aloud` 为外部目录。

## 安装命令

```bash
sudo dpkg -i dist/wps-read-aloud-xc_1.0.24_arm64.deb
```

如果 WPS 已经打开，请安装完成后重启 WPS。
