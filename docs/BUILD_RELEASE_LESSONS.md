# 构建与发布经验记录

本文档记录本项目构建、测试、同步和发布过程中已经验证过的失败动作、替代方案和固定流程，避免后续版本重复踩坑。

## Windows 本机构建

- 不使用 Windows Store 的 `python` 命令；它可能只是商店别名。需要运行项目脚本时使用 Codex 工作区内置 Python：
  `C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe`
- 需要执行 JavaScript 语法检查时使用 Codex 工作区内置 Node：
  `C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe`
- Go 服务目标环境是 Linux ARM64。Windows 上不能直接运行交叉编译后的测试二进制；验证方式应使用编译检查：
  `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o C:\tmp\wps-tts-daemon.test .\cmd\wps-tts-daemon`
- Windows 上构建 Linux ARM64 daemon 时使用 `-buildvcs=false`，避免 VCS 元信息写入在受限环境下失败。

## 前端与图标

- WPS Linux 加载项 Ribbon 图标优先使用 `size="large"` 配合 `getImage="ribbon.GetImage"`，静态 `image="..."` 在部分 WPS Linux 环境下可能不显示。
- 当前图标源文件使用 64x64 PNG RGBA 黑色图标，放在 `addin/assets/icons/`，同步脚本会复制到嵌入式服务目录。
- 仅修改前端、图标、弹窗样式或说明文件时，可以复用最近一个安装包里的 daemon 二进制，不必重新编译 Go 服务。

## 语音与性能

- 不要把逗号、顿号、冒号、分号等句内标点拆成多个 TTS 合成任务；这样会让一句话内的合成次数从 1 次变成多次，低性能机器上启动等待会明显增加。
- 推荐策略是“句内文本节奏提示 + 句末 WAV 精确静音”：每句仍只调用一次 fanchen-C 合成，生成后再追加句末静音。
- 默认 `1.2x` 语速下，句内标准停顿按约 `400ms` 设计，句末追加 `600ms` 静音；其他语速按比例缩放。

## 发布目录

- `dist/` 最终只保留本版本 `.deb` 和 `.sha256`。临时检查脚本、发布日志、旧安装包、可能包含认证信息的输出文件都应清理。
- `dist/wps-tts-daemon` 只是打包时复用或缓存的 daemon 二进制，不是最终交付物；发布前应清理，避免用户误用。

## GitHub 推送与 Release

- 使用长期复用脚本 `scripts/push_github.ps1` 和 `scripts/publish_github_release.ps1`，不再为每个版本生成一次性脚本。
- 推送和发布优先使用本机 Git Credential Manager。脚本不得输出 token、Basic 认证头或其他敏感凭据。
- 每次发布 GitHub Release 时，Release 内容要包含版本号、发布日期、主要变更、安装包文件名和 SHA256。
