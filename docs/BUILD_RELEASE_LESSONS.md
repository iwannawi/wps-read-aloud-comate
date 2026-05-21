# 构建与发布经验记录

本文档只记录已经验证过的失败动作和替代方案。正式安装包不包含本文档。

## 工具链

- 不使用 Windows Store 的 python 别名。运行项目脚本时使用 Codex 工作区内置 Python。
- 执行 JavaScript 语法检查时使用 Codex 工作区内置 Node。
- Windows PowerShell 5 会按本地编码误读无 BOM 的 UTF-8 脚本。Windows 安装、卸载脚本应保存为 UTF-8 BOM，并用 Windows PowerShell 5 做语法检查。
- 在 PowerShell 外层命令中包含 $null、$files、$f、$s 等变量时，避免外层提前展开。
- Go 服务必须在 daemon 目录内构建。Windows 安装器必须在 packaging/windows/installer 目录内测试。

## 构建

- 正式发布只使用 packaging/build_all.py 构建五个目标。
- 单目标构建只用于调试，不用于正式发布。
- 目标清单：windows、kylin-amd64、kylin-arm64、uos-amd64、uos-arm64。
- 在 Windows 本机交叉编译 Linux daemon 时使用 -buildvcs=false，避免受限环境读取 VCS 元信息失败。
- x86/x64 Windows 10/11 不能直接运行交叉编译后的 Linux 测试二进制。验证方式是多目标编译检查和本机可运行的 Go 单元测试。

## Windows 安装

- 默认安装目录使用 %LOCALAPPDATA%\Programs\WPS Read Aloud Comate，不写入 C:\Program Files。
- 使用 HKCU\Software\Microsoft\Windows\CurrentVersion\Run 注册当前用户自启动，不依赖计划任务。
- 旧版计划任务只做清理兼容。RunLevel 只允许 Limited 或 Highest，不使用 LeastPrivilege。
- 安装阶段直接 Start-Process 启动 daemon；登录自启动再使用 start-daemon.ps1。
- 安装前必须停止当前安装目录下正在运行的旧版 wps-tts-daemon.exe。
- 安装期健康检查等待 60 秒，避免低性能机器或首次杀毒扫描导致误判。
- 安装器应构建为 Windows GUI 子系统程序，正常安装不弹命令行窗口。
- Go 安装器启动 WinForms 界面时不要隐藏窗体。应隐藏控制台，保留安装界面。

## Windows WPS 注册

- 安装脚本不得覆盖整个 WPS 加载项配置文件，只增删本项目条目。
- 注册名称使用中文“文档朗读助手”。
- 授权描述使用“WPS文档朗读助手加载项申请访问本机语音合成服务”。
- Windows WPS 端只写 publish.xml 的单个 jspluginonline 入口，不再同时写 jsplugins.xml。
- Windows WPS 2023 当前依赖 publish.xml online 入口显示选项卡；重复写 online 和 local 会造成连续授权弹窗。
- Windows WPS 对 127.0.0.1 online 入口会生成一次安全确认，项目侧只能避免重复注册，不能伪造或关闭 WPS 安全确认。
- 升级时清理当前中文名称和旧内部名称的授权缓存、阻止缓存。
- x86 Windows 10/11 启动 PowerShell 时优先使用 Sysnative，避免漏读 64 位注册表。

## Linux 安装

- x64 银河麒麟 V10 及以上、ARM64 银河麒麟 V10 及以上、x64 UOS V20、ARM64 UOS V20 使用 systemd。
- 新版本使用 wps-read-aloud-comate.service，避免与旧版 wps-tts.service 发生文件归属冲突。
- 不要再通过 Conflicts/Replaces 强制移除旧包名；旧包维护脚本损坏时，dpkg 会先执行旧脚本并导致新包无法接管。
- 安装时应停用旧版 wps-tts.service，再启用 wps-read-aloud-comate.service。
- 同包名升级仍要处理旧版本残留：postinst 清理旧服务文件、旧注册脚本和已废弃的 piper/espeak 目录。
- 同版本重装时，如果端口已由本项目旧服务占用，preinst 不应直接阻断。
- 判断旧服务时同时检查 marker、service 文件路径和 /health 响应。
- WPS 首次访问 127.0.0.1:19860 的授权弹窗可能由 WPS 内核生成，不能完全依赖 desc 改写文案。

## 前端与弹窗

- WPS ShowDialog 使用 http://127.0.0.1:19860/dialog.html 的绝对地址。
- ShowDialog 失败后不要在 WPS 环境回退到 window.open，避免调起外部浏览器。
- 启动小窗如出现滚动条，需要同时检查 html.compact 和 body.compact 的最小尺寸。
- 部分 WPS 内置浏览器不支持 URLSearchParams，弹窗参数解析应使用兼容实现。

## 图标

- WPS Office for Linux Ribbon 图标优先使用 size="large" 配合 getImage。
- getImage 默认返回加载项静态资源路径，例如 assets/icons/start.png。
- Base64 和 data URI 在部分 WPS Linux 版本中可能显示问号，不作为默认方案。

## 语音引擎

- Windows Sherpa-onnx 使用 --vits-tokens，不使用 --tokens。
- Windows Sherpa-onnx 当前不支持 --tts-sample-rate。
- 如果出现“Sherpa-onnx 语音引擎启动失败”，先运行 payload 内 sherpa-onnx-offline-tts.exe --help，再用完整参数直接合成 WAV。
- Windows PowerShell 写配置文件可能带 UTF-8 BOM。Go 简易 YAML 解析器必须去掉行首 BOM，否则 listen 会回退到默认端口。
- 逗号、顿号、冒号、分号等句内标点不要拆成多个 TTS 任务。
- 推荐策略是句内文本节奏提示加句末 WAV 静音。

## 发布目录

- dist 最终只保留本版本五个安装包和五个 sha256 文件。
- 不保留临时检查脚本、发布日志、旧安装包和可能含认证信息的输出。
- dist/wps-tts-daemon 不是最终交付物，发布前清理。
- 清理 dist 时不要在外层 PowerShell 双引号中直接写 $dist、$_ 等变量；变量会被提前展开。使用反引号转义变量，或避免使用外层变量。
- Debian 包内容检查时不要硬编码 ./control；当前 tar 成员路径不带 ./ 前缀。

## GitHub

- 使用 scripts/push_github.ps1 和 scripts/publish_github_release.ps1，不再生成一次性脚本。
- 推送优先使用 GitHub CLI token，通过临时 http.extraHeader 注入本次 Git 命令。
- 如果 Git Credential Manager 凭据失效，不重复运行同一失败路径。
- 仓库重命名后，常驻推送脚本仍可能被旧 HTTPS 凭据带偏并报 “Invalid username or token”。此时直接读取 gh auth token，禁用 credential.helper，并用临时 http.extraHeader 推送。
- Release 发布优先使用 GitHub CLI；curl REST 路径返回 401 时不要反复重试。
- Release 内容只写当前版本新增、变更、修复、安装包和 SHA256。
