# WPS 文档朗读助手

目标环境：
- ARM64 麒麟操作系统
- WPS Office 2023 for Linux / WPS Office 2019 for Linux
- 允许安装 WPS JS 加载项
- WPS JS API 可读取 WPS 文字选区和全文
- 本机允许访问 `127.0.0.1`

本项目提供一个离线 WPS 文档朗读加载项。WPS 顶部新增“文档朗读”选项卡，用户可以从当前光标处开始朗读，支持连页朗读、当页朗读、语速选择、状态检查和关于说明。
默认 `1.2x` 语速下，句内语义标点按约 `400ms` 节奏停顿，句末追加约 `600ms` 静音；其他语速下停顿时长随语速等比例缩放。

## 项目结构

```text
addin/                         WPS JS 加载项源文件
daemon/                        Go 本地服务，监听 127.0.0.1:19860
packaging/deb/                 企业交付用 .deb 打包脚本和 Debian 控制脚本
packaging/sync_addin_web.py    同步 addin/ 到 Go embed 目录
third_party_licenses/          第三方许可证和声明
engines/                       打包时放置 Sherpa-onnx 运行文件
voices/                        打包时放置离线语音模型
```

## 工作方式

```text
WPS 文字 -> 文档朗读选项卡 -> http://127.0.0.1:19860 -> Go 服务 -> Sherpa-onnx VITS fanchen-C -> 系统播放器
```

服务只监听 `127.0.0.1:19860`，不访问外网。当前版本统一使用 `vits-zh-hf-fanchen-C` 中文 VITS 模型。英文和数字会在文本预处理阶段转换为逐字符中文读法，例如 `WPS 2026` 会逐字符读出，避免模型跳过英文或数字。

朗读时按完整语句切分、逐句合成并播放；加载项会在 WPS 文档中同步选中当前朗读语句，并尽量保持当前语句可见。启动朗读时，服务按句动态预处理，累计文本达到约 100 字即可开始播放；如果第一句已超过 100 字，只等待第一句合成完成，减少启动等待。

## 准备离线依赖

打包前需要准备：

```text
engines/sherpa-onnx/sherpa-onnx-offline-tts
engines/sherpa-onnx/lib/
voices/sherpa/vits-zh-hf-fanchen-C/
```

这些文件会被打入 `.deb`，安装到 `/opt/wps-read-aloud/engines` 和 `/opt/wps-read-aloud/voices`。

## 构建

先同步加载项到 Go embed 目录：

```bash
python3 packaging/sync_addin_web.py
```

交叉编译 Linux ARM64 服务后，生成安装包：

```bash
python3 packaging/deb/build_deb.py
```

最终交付文件：

```text
dist/wps-read-aloud-xc_1.0.21_arm64.deb
```

## 安装

在 ARM64 麒麟目标机执行：

```bash
sudo dpkg -i dist/wps-read-aloud-xc_1.0.21_arm64.deb
```

安装包会：
- 安装程序文件到 `/opt/wps-read-aloud`
- 安装配置文件到 `/etc/wps-read-aloud/config.yaml`
- 安装并启动 `wps-tts.service`
- 为已有普通用户注册 WPS JS 加载项
- 写入安装日志 `/var/log/wps-read-aloud-install.log`
- 安装第三方组件许可证和交付说明到 `/usr/share/doc/wps-read-aloud-xc/`

如果 WPS 已打开，安装后需要重启 WPS 才能加载新版“文档朗读”选项卡。

## 验证

```bash
systemctl status wps-tts.service --no-pager
curl http://127.0.0.1:19860/health
curl http://127.0.0.1:19860/selftest
```

打开 WPS 文字后，顶部应出现“文档朗读”选项卡。优先点击“状态检查”，如果弹出服务状态提示，说明 Ribbon 按钮回调已正常触发。

## 版本管理

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
