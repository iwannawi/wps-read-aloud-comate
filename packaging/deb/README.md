# DEB 打包说明

目标交付文件：

```text
dist/wps-read-aloud-zhangjingyao_1.0.14_arm64.deb
```

## 唯一规范打包入口

统一使用：

```bash
python3 packaging/deb/build_deb.py
```

兼容入口：

```bash
./packaging/deb/build-deb.sh
```

Windows 本机兼容入口：

```powershell
.\packaging\deb\build-deb.ps1
```

上述兼容入口都会转调 `build_deb.py`。不要再维护多套独立打包逻辑。

## 必需输入

构建 `.deb` 前需要准备：

```text
dist/wps-tts-daemon
engines/sherpa-onnx/sherpa-onnx-offline-tts
engines/sherpa-onnx/lib/
voices/sherpa/vits-zh-hf-fanchen-C/
```

`build_deb.py` 会强制校验上述文件，缺任意一项都会失败，避免生成“装得上但不能朗读”的安装包。

## 构建流程

推荐流程：

```bash
chmod +x packaging/kylin/build-arm64.sh packaging/deb/build-deb.sh
./packaging/kylin/build-arm64.sh
python3 packaging/deb/build_deb.py
```

`build-arm64.sh` 会同步 `addin/` 到 Go embedded web 目录，再编译 `dist/wps-tts-daemon`。

`build_deb.py` 会在打包前校验 `addin/` 和 `daemon/cmd/wps-tts-daemon/web/` 是否完全一致；不一致时直接失败。

## 安装

```bash
sudo dpkg -i dist/wps-read-aloud-zhangjingyao_1.0.14_arm64.deb
```

安装后会：

- 安装 `/opt/wps-read-aloud`
- 安装并启动系统服务 `wps-tts.service`
- 为已有普通用户写入 WPS 加载项注册文件
- 提供 `wps-read-aloud-register` 用于后续用户或注册修复
- 写入安装/卸载日志：`/var/log/wps-read-aloud-install.log`
- 安装第三方组件许可证文件：`/usr/share/doc/wps-read-aloud-zhangjingyao/`
- 安装发布说明、验收测试说明、源码获取说明、校验说明到同一文档目录

## 验证

```bash
systemctl status wps-tts.service --no-pager
curl http://127.0.0.1:19860/health
curl http://127.0.0.1:19860/selftest
wps-read-aloud-register
```

重启 WPS 后查看顶部“文档朗读”选项卡。
