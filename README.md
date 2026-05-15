# WPS 文档朗读加载项

目标环境：

- 银河麒麟 V10 ARM64
- WPS 2023 for Linux 12.1.x
- 允许安装 WPS JS 加载项
- WPS JS API 可读取 WPS 文字选区和全文
- 本机允许访问 `127.0.0.1`

本项目采用：

- `addin/`：WPS JS 加载项源文件，在 WPS 顶部新增“文档朗读”选项卡。
- `daemon/`：Go 本地服务，只监听 `127.0.0.1:19860`。
- `packaging/deb/`：企业交付用 `.deb` 打包脚本和 Debian 控制脚本。
- `packaging/sync_addin_web.py`：把 `addin/` 同步到 Go embedded web 目录，避免嵌入资源和源加载项不一致。

## 工作方式

```text
WPS 文字 -> 文档朗读选项卡 -> http://127.0.0.1:19860 -> Go 服务 -> Sherpa-onnx Matcha 双模型 -> 系统播放器播放音频
```

Sherpa-onnx 是唯一离线语音合成引擎。中文片段使用 `matcha-icefall-zh-baker`，英文片段使用 `matcha-icefall-en_US-ljspeech`，两者共用 `vocos-22khz-univ.onnx` 声码器，原生输出 `22050 Hz` 单声道 WAV。服务会用正则表达式把中英文混合文本切分成片段，分别调用对应模型合成，再把音频无重采样拼接成一段完整音频。播放时优先使用已经探测成功的系统播放器；如果还没有探测结果，会依次尝试系统已有的 `pw-play`、`paplay` 或 `aplay`。所有文本只发送到本机回环地址，不访问外网。

朗读时会按完整语句切分、逐句合成并播放；加载项会在 WPS 文档中选中当前朗读语句，进入下一句时同步选中下一句。顶部选项卡提供“全文朗读、当前位置朗读、选区朗读”三种入口。低配置机器上建议优先使用“选区朗读”或“当前位置朗读”，加载项会限制单次句子数量和单句长度，避免长文档造成长时间等待或资源占用过高。

## 目录

```text
addin/
  manifest.xml
  ribbon.xml
  index.html
  assets/
daemon/
  cmd/wps-tts-daemon/main.go
  config.example.yaml
packaging/
  sync_addin_web.py
  deb/
  kylin/
third_party_licenses/
voices/
  .gitkeep
```

## 构建本地服务

在项目根目录执行：

```bash
chmod +x packaging/kylin/build-arm64.sh
./packaging/kylin/build-arm64.sh
```

该脚本会先同步 `addin/` 到 `daemon/cmd/wps-tts-daemon/web/`，再交叉编译 Linux ARM64 服务：

```text
dist/wps-tts-daemon
```

## 准备离线依赖

打包前必须准备：

```text
engines/sherpa-onnx/sherpa-onnx-offline-tts
engines/sherpa-onnx/lib/
voices/sherpa/matcha-icefall-zh-baker/
voices/sherpa/matcha-icefall-en_US-ljspeech/
voices/sherpa/vocos-22khz-univ.onnx
```

这些文件会被打入 `.deb`，安装到 `/opt/wps-read-aloud/engines` 和 `/opt/wps-read-aloud/voices`。

## 生成 DEB 安装包

统一使用 Python 打包入口：

```bash
python3 packaging/deb/build_deb.py
```

兼容脚本 `packaging/deb/build-deb.sh` 和 `packaging/deb/build-deb.ps1` 也会转调同一个 `build_deb.py`，避免不同脚本生成不同安装包。

最终交付文件：

```text
dist/wps-read-aloud-zhangjingyao_1.0.10_arm64.deb
```

## 安装

在银河麒麟 V10 ARM64 目标机执行：

```bash
sudo dpkg -i dist/wps-read-aloud-zhangjingyao_1.0.10_arm64.deb
```

安装包会：

- 安装 `/opt/wps-read-aloud`
- 安装 `/etc/wps-read-aloud/config.yaml`
- 安装并启动系统服务 `wps-tts.service`
- 覆盖升级时重启 `wps-tts.service`，避免 WPS 加载项与旧版后台服务不匹配
- 安装时探测当前环境可用播放器，并保存到 `/var/lib/wps-read-aloud/audio-player.json`
- 为已有普通用户注册 WPS JS 加载项
- 写入安装日志 `/var/log/wps-read-aloud-install.log`
- 安装第三方组件许可证和交付说明到 `/usr/share/doc/wps-read-aloud-zhangjingyao/`

如果 WPS 已打开，需要重启 WPS 后才能加载新的“文档朗读”选项卡。

## 验证

```bash
systemctl status wps-tts.service --no-pager
curl http://127.0.0.1:19860/health
curl http://127.0.0.1:19860/selftest
```

打开 WPS 文字后，顶部应出现“文档朗读”选项卡。优先点击“状态检查”，如果弹出服务状态提示，说明 Ribbon 按钮回调已经正常触发。

## API

```http
GET /health
GET /selftest
GET /audio/probe
POST /audio/probe
POST /play
POST /synthesize
POST /speak
POST /stop
POST /pause
POST /resume
GET /voices
```

加载项默认使用 `POST /play` 由本地服务完成系统侧播放，避免 WPS 内置浏览器自动播放限制；`POST /synthesize` 保留为 WAV 合成接口，`POST /speak` 作为兼容别名保留。

## Git 管理

源码、脚本、配置、文档和许可证进入 Git。以下内容不进入普通 Git：

- `dist/`
- `engines/`
- `tools/`
- `voices/sherpa/`
- 构建缓存和下载缓存

详细版本管理规则见：

```text
docs/GIT_WORKFLOW.md
docs/CODEX_AUTOMATION.md
```
