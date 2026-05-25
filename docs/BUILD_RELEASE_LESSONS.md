# 构建与发布经验记录

本文档只记录已经验证过的失败动作和替代方案。正式安装包不包含本文档。

## 工具链

- 不使用 Windows Store 的 python 别名。运行项目脚本时使用 Codex 工作区内置 Python。
- 不使用裸 python 执行同步、构建、版本替换等项目脚本。Windows Store 别名会失败，应直接调用 Codex 工作区内置 Python。
- 执行 JavaScript 语法检查时使用 Codex 工作区内置 Node。
- Windows PowerShell 5 会按本地编码误读无 BOM 的 UTF-8 脚本。Windows 安装、卸载脚本应保存为 UTF-8 BOM，并用 Windows PowerShell 5 做语法检查。
- 不用 Windows PowerShell 5 的 Get-Content/Set-Content 批量改写中文 Markdown、XML、JS 文件；它会把 UTF-8 中文误读成乱码。批量文本替换必须使用显式 UTF-8 读写。
- 在 PowerShell 外层命令中包含 $null、$files、$f、$s 等变量时，避免外层提前展开。
- Go 服务必须在 daemon 目录内构建。Windows 安装器必须在 packaging/windows/installer 目录内测试。

## 构建

- 正式发布只使用 packaging/build_all.py 构建五个目标。
- 正式全目标构建前清理 build/deb、build/windows、auth-test、launcher-test 和临时 .test 文件，避免旧版本解包目录或测试二进制干扰发版检查。
- 单目标构建只用于调试，不用于正式发布。
- 目标清单：windows、kylin-amd64、kylin-arm64、uos-amd64、uos-arm64。
- 在 Windows 本机交叉编译 Linux daemon 时使用 -buildvcs=false，避免受限环境读取 VCS 元信息失败。
- x86/x64 Windows 10/11 不能直接运行交叉编译后的 Linux 测试二进制。验证方式是多目标编译检查和本机可运行的 Go 单元测试。

## Windows 安装

- 默认安装目录使用 %LOCALAPPDATA%\Programs\WPS Read Aloud Comate，不写入 C:\Program Files。
- Windows 版本不写入 HKCU\Software\Microsoft\Windows\CurrentVersion\Run，不创建计划任务，不在安装后启动 daemon。选项卡加载依赖 OEM + publish 离线目录，不依赖 daemon 常驻。
- 旧版计划任务只做清理兼容。RunLevel 只允许 Limited 或 Highest，不使用 LeastPrivilege。
- 安装阶段只生成 start-daemon.ps1，供 WPS 加载项按需启动服务。
- 安装前必须停止当前安装目录下正在运行的旧版 wps-tts-daemon.exe。
- 安装期不做本地服务健康检查，避免为了检查而启动后台进程。
- Windows 安装包必须写入开始菜单卸载入口和当前用户“应用和功能”卸载注册表。
- Windows 卸载脚本必须停止本项目 daemon，清理旧版 Run 自启动项、旧计划任务、WPS 加载项注册、授权缓存、开始菜单入口、卸载注册表和安装目录。
- 安装器应构建为 Windows GUI 子系统程序，正常安装不弹命令行窗口。
- Go 安装器启动 WinForms 界面时不要隐藏窗体。应隐藏控制台，保留安装界面。

## Windows WPS 注册

- 安装脚本不得覆盖整个 WPS 加载项配置文件，只增删本项目条目。
- 注册名称使用中文“文档朗读助手”。
- 授权描述使用“WPS文档朗读助手加载项申请访问本机语音合成服务”。
- Windows WPS 端使用 OEM + publish 离线模式：复制“文档朗读助手_版本号”目录，生成 jsplugins.xml，并将 oem.ini 的 JSPluginsServer 指向本地 jsplugins.xml。
- Windows WPS 2019 离线模式需要 oem.ini 中的 disableFileCheckIntercept=true，否则可能无法完整生成或加载离线加载项。
- 离线 jsplugin 的 url 必须指向可访问的 .7z 压缩包。只预置 name_version 目录但不生成真实 .7z 时，部分 Windows WPS 版本会看不到选项卡。Windows 10/11 可用系统自带 tar.exe 生成 7z 包，安装脚本必须校验 7z 文件头。
- Windows 端停止朗读不能调用 /shutdown。停止朗读只调用 /read/stop，终止当前会话、播放和 sherpa-onnx 子进程。
- 本机 Windows 环境中 python 命令可能被 Microsoft Store 应用执行别名接管，py 命令也可能不可用。构建和同步脚本优先使用 Codex 运行时 Python：C:\Users\zhangjingyao\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe。
- Windows HTTP 根地址模式已废弃。它要求 WPS 打开时本地服务仍在运行，容易导致安装后稍晚打开 WPS 看不到选项卡。
- 不再写 publish.xml online 入口；旧 publish.xml 内本项目条目只清理，不新增。
- Windows WPS 对 127.0.0.1 online 入口可能生成一次原生安全确认，项目侧只能避免重复注册，不能伪造或关闭 WPS 安全确认。
- 升级时不要清理已允许的 authaddin.json 授权缓存；应保留并刷新本项目条目，避免每次升级后重复弹出许可确认。
- 只清理本项目相关的阻止缓存和重复 publish/jsplugins 条目；不处理其他加载项的授权信息。
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

- Linux WPS ShowDialog 使用 http://127.0.0.1:19860/dialog.html 的绝对地址。Windows 本地加载项使用本地 dialog.html，避免为了弹窗启动本地服务。
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
- Windows 短文档可朗读但长文档启动失败时，优先检查启动预合成策略、并发合成数量、单句异常字符和系统资源压力，不要直接判断为安装包损坏。
- Windows 播放链路应使用原生 WinMM 播放 WAV，避免每句朗读都启动 PowerShell 播放进程。
- Windows 播放停止不能使用同步 PlaySound 等待当前 WAV 自然结束。应使用异步 PlaySound，并在停止请求到达时调用 PlaySound(NULL) 中断当前声音。
- 不要把复杂的 Windows 包内播放停止测试写成超长 PowerShell 内联命令；本地执行器可能中断且难以诊断。应落成固定测试脚本后再运行。
- Windows PowerShell 写配置文件可能带 UTF-8 BOM。Go 简易 YAML 解析器必须去掉行首 BOM，否则 listen 会回退到默认端口。
- 长文档预合成不能只按累计字数判断。短句文档会在启动阶段并发创建大量 Sherpa 进程，必须同时设置每轮预合成句数上限。
- 逗号、顿号、冒号、分号等句内标点不要拆成多个 TTS 任务。
- 推荐策略是句内文本节奏提示加句末 WAV 静音。
- 数学百分数要先于 ASCII 逐字符规则处理。例如 10% 应转换为“百分之十”，不能先变成“一 零 百分号”。
- 固定办公术语要先于通用英文逐字符规则处理。例如 WPS 读作“达不溜屁挨思”，Office 和 office 读作“凹斐思”。

## 发布目录

- dist 最终只保留本版本五个安装包和五个 sha256 文件。
- 不保留临时检查脚本、发布日志、旧安装包和可能含认证信息的输出。
- 发布前如果 dist/github-push.log 仍存在，verify_release_artifacts.py 会阻断 Release；推送成功后应删除该日志再发布。
- dist/wps-tts-daemon 不是最终交付物，发布前清理。
- 清理 dist 时不要在外层 PowerShell 双引号中直接写 $dist、$_ 等变量；变量会被提前展开。使用反引号转义变量，或避免使用外层变量。
- Debian 包内容检查时不要硬编码 ./control；当前 tar 成员路径不带 ./ 前缀。

## GitHub

- 使用 scripts/push_github.ps1 和 scripts/publish_github_release.ps1，不再生成一次性脚本。
- 推送优先使用 GitHub CLI token，通过临时 http.extraHeader 注入本次 Git 命令。
- GitHub CLI token 通过只读 ls-remote 校验不代表具有 push 权限；如果 push 返回 “Invalid username or token”，不要反复重试同一个 token，应改用具备 Contents 读写权限的 token 或重新授权。
- 如果 Git Credential Manager 凭据失效，不重复运行同一失败路径。
- 仓库重命名后，常驻推送脚本仍可能被旧 HTTPS 凭据带偏并报 “Invalid username or token”。此时直接读取 gh auth token，禁用 credential.helper，并用临时 http.extraHeader 推送。
- Release 发布优先使用 GitHub CLI；curl REST 路径返回 401 时不要反复重试。
- Release 内容只写当前版本新增、变更、修复、安装包和 SHA256。
