# 银河麒麟 V10 ARM64 部署清单

## 1. 目标环境

- 银河麒麟 V10 ARM64
- WPS 2023 for Linux 12.1.x
- 允许安装 WPS JS 加载项
- 允许访问 `127.0.0.1:19860`

## 2. 推荐安装方式

企业交付优先使用 `.deb`：

```bash
sudo dpkg -i dist/wps-read-aloud-zhangjingyao_1.0.14_arm64.deb
```

安装后会自动：

- 安装 `/opt/wps-read-aloud`
- 安装 `/etc/wps-read-aloud/config.yaml`
- 安装并启动 `wps-tts.service`
- 为已有普通用户注册 WPS 加载项
- 写入日志 `/var/log/wps-read-aloud-install.log`

如果安装前 WPS 已打开，需要重启 WPS。

## 3. 服务检查

```bash
systemctl status wps-tts.service --no-pager
curl http://127.0.0.1:19860/health
curl http://127.0.0.1:19860/selftest
```

`/health` 应返回可用引擎，`/selftest` 应能生成测试音频。

## 4. WPS 验收

打开 WPS 文字后，顶部应出现“文档朗读”选项卡。

验收点：

- 能看到“文档朗读”选项卡。
- “朗读选区”能读取当前选中内容。
- “朗读全文”能读取当前文档正文。
- “暂停”“继续”“停止”按钮可用。
- “状态检查”能弹出本地服务状态。
- 朗读时 WPS 文档中当前语句会被选中，进入下一句时同步更新选区。
- 服务接口只监听 `127.0.0.1:19860`。
- 断网状态下 Sherpa-onnx 中文/英文离线模型能正常工作。

## 5. 手工构建流程

如果需要重新构建交付包：

```bash
chmod +x packaging/kylin/build-arm64.sh packaging/deb/build-deb.sh
./packaging/kylin/build-arm64.sh
python3 packaging/deb/build_deb.py
```

构建前需要准备：

```text
engines/sherpa-onnx/sherpa-onnx-offline-tts
engines/sherpa-onnx/lib/
voices/sherpa/vits-zh-hf-fanchen-C/
```

`build-arm64.sh` 会同步 `addin/` 到 Go embedded web 目录；`build_deb.py` 会在打包前再次校验同步状态。

## 6. 加载项注册修复

如果安装后新建了 Linux 用户，或 WPS 加载项注册文件被清理，可在对应用户下执行：

```bash
wps-read-aloud-register
```

如果要为所有普通用户重新注册：

```bash
sudo wps-read-aloud-register --all-users
```
