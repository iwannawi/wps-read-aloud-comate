# 构建与发布经验记录

本文档记录本项目构建、测试、同步和发布过程中已经验证过的失败动作、替代方案和固定流程，避免后续版本重复踩坑。

## Windows 本机构建

- 不使用 Windows Store 的 “python” 命令；它可能只是商店别名。需要运行项目脚本时使用 Codex 工作区内置 Python：
  “C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe”
- 2026-05-19 再次验证：直接运行 “python packaging\sync_addin_web.py” 会触发 Windows Store Python 提示，仍应使用 Codex 工作区内置 Python。
- 需要执行 JavaScript 语法检查时使用 Codex 工作区内置 Node：
  “C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe”
- 在 PowerShell 里用 “powershell.exe -Command” 检查脚本语法时，不要把含有 “$null” 的命令放在外层双引号里；外层 PowerShell 会提前展开变量，导致出现 “=[scriptblock]::Create” 这类误报。推荐直接使用 “[scriptblock]::Create(... ) | Out-Null”。
- Windows PowerShell 5 读取无 BOM 的 UTF-8 脚本时可能按本地编码解析，中文字符串会乱码，严重时会造成脚本解析失败。Windows 安装、卸载脚本需要保存为带 BOM 的 UTF-8，并用 Windows PowerShell 5 做语法检查。
- 在外层 PowerShell 调用 “powershell.exe -Command” 时，含 “$files”、“$f”、“$s” 的命令要放在单引号中，避免外层提前展开变量导致 “foreach( in )” 这类语法错误。
- Go 服务目标环境包含 Windows x86、本地 Linux amd64 和 Linux arm64。Windows 上不能直接运行交叉编译后的 Linux 测试二进制；验证方式应使用多目标编译检查和本机可运行的 Go 单元测试。
- Windows 上构建 Linux ARM64 daemon 时使用 “-buildvcs=false”，避免 VCS 元信息写入在受限环境下失败。
- Go 服务必须在 “daemon” 目录内构建。不要在仓库根目录直接运行 “go build .\daemon\cmd\wps-tts-daemon”，否则会出现 “cannot find main module, but found .git/config”。
- 在 “daemon” 目录内运行 Go 命令时，PowerShell 下不要写 “tools\go\bin\go.exe”；这会被当作模块名解析。应使用相对父目录形式 “..\tools\go\bin\go.exe” 或完整路径。仓库根目录也不是 Go module 根，Windows 安装器模块需要进入 “packaging\windows\installer” 后用 “..\..\..\tools\go\bin\go.exe test .” 验证。
- 本机已验证的 Linux ARM64 daemon 构建方式是：先进入 “daemon” 目录，再执行 “GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE=C:\tmp\go-build-cache ..\tools\go\bin\go.exe build -buildvcs=false -o ..\dist\wps-tts-daemon-linux-arm64 .\cmd\wps-tts-daemon”。

## 多平台安装包

- 多平台入口脚本是 “packaging\build_all.py”。使用 “--list” 查看五类安装包目标；按需调试单个目标时使用对应目标编号，例如 “windows”、“kylin-amd64”、“kylin-arm64”、“uos-amd64” 或 “uos-arm64”。
- 当前仓库已经具备五类安装包的目录、配置和脚本框架；正式发布前必须确认每个目标的 Sherpa ONNX 运行文件、模型资源和 daemon 二进制都已准备齐全。
- Windows 安装脚本不得覆盖整个 WPS 加载项配置文件。安装和卸载时只增删 “wps-read-aloud” 条目，并对原配置文件生成带时间戳的备份。
- Windows 安装包默认不要写入 “C:\Program Files (x86)”。普通用户无管理员权限时会报 “New-Item：访问被拒绝”。默认安装路径应使用 “%LOCALAPPDATA%\Programs\WPS Read Aloud Comate”，日志使用 “%LOCALAPPDATA%\WPSReadAloudComate\Logs”。
- Windows 计划任务的 “RunLevel” 只能使用 “Limited” 或 “Highest”。不要使用 “LeastPrivilege”，否则部分系统会在安装时报参数转换失败。
- Windows 安装前必须探测 WPS 客户端：至少检查 “wps.exe” 路径、产品版本和 PE 位数。本项目不是进程内 DLL 插件，WPS JS 加载项通过 127.0.0.1 调用独立本地朗读服务，因此本地服务位数不需要和 WPS 位数一致；位数检测用于日志和故障定位，不应阻止 64 位 WPS 安装。
- Windows x86 安装器启动 “powershell.exe” 时可能进入 32 位 PowerShell，从而漏读 64 位注册表和 “C:\Program Files”。安装器应优先调用 “%WINDIR%\Sysnative\WindowsPowerShell\v1.0\powershell.exe”，安装脚本也必须读取 “App Paths\wps.exe”、Kingsoft/WPS 注册表键、开始菜单快捷方式和 “ProgramW6432”。
- Windows 安装器应使用 “go build -ldflags -H=windowsgui” 构建为 GUI 子系统程序，避免正常安装时弹出命令行窗口。失败和成功提示使用系统消息框。
- 正式发布必须运行 “python packaging/build_all.py” 构建五个目标，并运行 “python packaging/verify_release_artifacts.py” 检查五个安装包。只构建单个目标只能用于本地调试，不能用于发布 Release。
- 发布脚本 “scripts\publish_github_release.ps1” 必须在上传前执行五包完整性检查；缺少任意一个安装包或 SHA256 文件时立即失败。
- 安装包资源要按平台最小化：Linux 包不得包含 Windows exe、Piper、eSpeak NG；Windows 包不得包含 Linux systemd 服务、Linux so、Piper、eSpeak NG。

## 前端与图标

- WPS Linux 加载项 Ribbon 图标优先使用 “size="large"” 配合 “getImage="ribbon.GetImage"”，静态 “image="..."” 在部分 WPS Linux 环境下可能不显示。
- 当前图标源文件放在 “addin/assets/icons/”；WPS Ribbon 回调应返回加载项内静态资源路径，例如 “assets/icons/start.png”。已验证 Base64 和 data URI 在部分 WPS Linux 版本中可能显示问号占位图，不再作为默认方案。
- 仅修改前端、图标、弹窗样式或说明文件时，可以复用最近一个安装包里的 daemon 二进制，不必重新编译 Go 服务。

## 语音与性能

- 不要把逗号、顿号、冒号、分号等句内标点拆成多个 TTS 合成任务；这样会让一句话内的合成次数从 1 次变成多次，低性能机器上启动等待会明显增加。
- 推荐策略是“句内文本节奏提示 + 句末 WAV 精确静音”：每句仍只调用一次 fanchen-C 合成，生成后再追加句末静音。
- 默认 “1.2x” 语速下，句内标准停顿按约 “400ms” 设计，句末追加 “600ms” 静音；其他语速按比例缩放。

## 发布目录

- “dist/” 最终只保留本版本 Windows “.exe”、Linux “.deb” 以及对应 “.sha256”。临时检查脚本、发布日志、旧安装包、可能包含认证信息的输出文件都应清理。
- “dist/wps-tts-daemon” 只是打包时复用或缓存的 daemon 二进制，不是最终交付物；发布前应清理，避免用户误用。
- 检查 Debian 包内容时，当前构建脚本生成的 tar 成员路径不带 “./” 前缀。控制文件路径是 “control”；银河麒麟数据文件示例是 “opt/wps-read-aloud-comate/version.json”，UOS 数据文件示例是 “opt/apps/cn.wps-read-aloud-comate/files/version.json”。检查脚本不要硬编码 “./control”。
- 如果刚运行过 Windows 安装包，再次构建可能因为安装器窗口仍在运行而无法覆盖 “dist” 下的 exe，错误表现为 “PermissionError: WinError 5”。先确认安装器窗口关闭，或检查并结束对应安装器进程后再重建。

## GitHub 推送与 Release

- 使用长期复用脚本 “scripts/push_github.ps1” 和 “scripts/publish_github_release.ps1”，不再为每个版本生成一次性脚本。
- 推送和发布优先使用本机 Git Credential Manager。脚本不得输出 token、Basic 认证头或其他敏感凭据。
- 2026-05-19 验证：如果 Git Credential Manager 里保存的 GitHub HTTPS 凭据失效，即使 “gh auth status” 正常，直接 “git push origin main” 仍会报 “Invalid username or token”。替代方案是优先读取 “gh auth token”，通过临时 “http.extraHeader” 注入本次 Git 命令，同时设置 “credential.helper=” 禁用失效凭据干扰，并避免在日志里打印认证头。
- 每次发布 GitHub Release 时，Release 内容要包含版本号、发布日期、主要变更、安装包文件名和 SHA256。
