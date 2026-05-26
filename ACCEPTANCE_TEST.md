# 验收测试说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.19

## 环境矩阵

| CPU 架构 + 操作系统 | WPS 要求 | 安装包 |
| --- | --- | --- |
| x86/x64 Windows 10/11 | WPS Office 2019 或更高版本 | wps-read-aloud-comate_1.1.19_windows.exe |
| x64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本 | wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | WPS Office 2019 for Linux 或更高版本 | wps-read-aloud-comate_1.1.19_arm64.deb |
| x64 UOS V20 | WPS Office 2019 for Linux 或更高版本 | cn.wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 UOS V20 | WPS Office 2019 for Linux 或更高版本 | cn.wps-read-aloud-comate_1.1.19_arm64.deb |

## 安装验收

| CPU 架构 + 操作系统 | 操作 |
| --- | --- |
| x86/x64 Windows 10/11 | 运行 wps-read-aloud-comate_1.1.19_windows.exe |
| x64 银河麒麟 V10 及以上 | sudo dpkg -i wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 银河麒麟 V10 及以上 | sudo dpkg -i wps-read-aloud-comate_1.1.19_arm64.deb |
| x64 UOS V20 | sudo dpkg -i cn.wps-read-aloud-comate_1.1.19_amd64.deb |
| ARM64 UOS V20 | sudo dpkg -i cn.wps-read-aloud-comate_1.1.19_arm64.deb |

预期结果：

- 安装过程无错误退出。
- Windows 安装器能显示安装进度、当前动作、安装步骤日志和最终结果。
- Windows 安装器不显示安装路径输入框，主要空间用于展示安装步骤和日志信息。
- Windows 安装日志写入 %LOCALAPPDATA%\WPSReadAloudComate\Logs\install.log。
- Windows 写入当前用户 Run 自启动项，安装后本机朗读服务应立即启动。
- Windows 安装完成后提示彻底退出并重新打开 WPS；重新打开后 WPS 应能通过 publish.xml 显示“文档朗读”选项卡。
- Windows 开始菜单存在卸载入口，系统“应用和功能”中存在本软件卸载项。
- Windows 当前用户 WPS jsaddons 目录下应存在 publish.xml，且包含 http://127.0.0.1:19860/addin/。
- Linux systemd 服务 wps-read-aloud-comate.service 启动并保持运行。
- Linux 安装日志写入 /var/log/wps-read-aloud-install.log。
- 覆盖安装同版本或升级安装时不发生文件覆盖冲突。
- 安装后重启 WPS，可看到“文档朗读”选项卡。

## 平台差异验收

| 验收项 | x86/x64 Windows 10/11 | x64/ARM64 银河麒麟 V10 及以上 | x64/ARM64 UOS V20 |
| --- | --- | --- | --- |
| 安装界面 | exe 图形安装界面显示进度、当前动作和日志，不弹命令行窗口 | 使用系统 deb 安装流程 | 使用系统 deb 安装流程 |
| 安装目录 | 默认位于当前用户目录，不要求管理员写入 Program Files | /opt/wps-read-aloud-comate | /opt/apps/cn.wps-read-aloud-comate/files |
| 服务管理 | publish.xml 模式加载选项卡；本机服务常驻并写入 Run 自启动；语音合成子进程只在朗读时运行 | systemd 管理 wps-read-aloud-comate.service | systemd 管理 wps-read-aloud-comate.service |
| 播放层 | Windows WinMM，可中断当前 WAV 播放 | pw-play、paplay、aplay 按环境探测 | pw-play、paplay、aplay 按环境探测 |
| WPS 许可确认 | 首次信任时可能出现 Windows WPS 原生确认框，点击允许后升级安装不应主动清除允许记录 | 通常不出现 Windows 同款确认框 | 通常不出现 Windows 同款确认框 |
| 日志 | %LOCALAPPDATA%\WPSReadAloudComate\Logs\install.log | /var/log/wps-read-aloud-install.log 和 journalctl | /var/log/wps-read-aloud-install.log 和 journalctl |

Windows 原生加载项许可确认框由 WPS 客户端安全策略生成，不属于本项目弹窗。验收时只确认名称、来源和升级后不重复清理授权缓存，不要求安装包绕过该安全确认。

## Windows 卸载与驻留验收

- 安装完成后应存在 wps-tts-daemon.exe，用于向 WPS 发布加载项和处理朗读请求。
- 打开 WPS 但未朗读时，不应存在 sherpa-onnx-offline-tts.exe 等语音合成子进程。
- 点击“开始朗读”后，可短时出现 sherpa-onnx-offline-tts.exe 语音合成子进程。
- 点击“停止朗读”或朗读完成后，语音合成子进程应退出；本地发布服务保持运行，供 WPS 加载项继续访问。
- 开始菜单“WPS文档朗读助手”目录下应有“卸载 WPS文档朗读助手”入口。
- 系统“应用和功能”中应显示“WPS 文档朗读助手”，并可执行卸载。
- 卸载后应停止本机服务，并清理安装目录、WPS 加载项条目、授权缓存、Run 自启动项、旧计划任务、开始菜单入口、卸载注册表项和本项目写入的 OEM 残留指向项。

## 选项卡验收

1. 打开 WPS 文字。
2. 确认顶部出现“文档朗读”选项卡。
3. 确认按钮为：开始朗读、停止朗读、朗读方式、朗读语速、状态检查、关于朗读。
4. 未朗读时，“停止朗读”置灰。
5. 朗读中，“开始朗读”“朗读方式”“朗读语速”“状态检查”“关于朗读”置灰，“停止朗读”可用。
6. “朗读方式”包含“连页朗读”和“当页朗读”，默认“连页朗读”。
7. “朗读语速”包含 0.75x、1x、1.2x、1.5x，默认 1.2x。
8. 任何按钮不应弹出“未知按钮”。

## 完整朗读验收

- 每次版本测试固定使用 D:\Dev Projects\维护手册.docx 做功能测试文档。该文档仅用于本地测试，不进入安装包和源码仓库。
- 每次版本测试必须覆盖完整文档，不能只抽测前 5 页，也不能只检查语音合成是否成功。
- 测试必须覆盖连页朗读、当页朗读、从文档开头朗读、从其他页当前光标处朗读、目录页朗读、正文页朗读和跨页长句朗读。
- 自动化检查命令：

    powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\test_wps_selection_consistency.ps1 -DocumentPath "D:\Dev Projects\维护手册.docx" -SummaryOnly

  如当前测试机的 WPS COM 会话被弹窗或外部进程阻塞，可临时增加 -PreferWordAutomation 参数，使用同一套 Range、Selection、Page 对象模型完成一致性回归。

- 连页朗读：有光标时从光标处读到文档末尾；无可识别光标时从文档开头读到文档末尾。
- 当页朗读：有光标时从光标处读到当前页末尾；无可识别光标时从当前页开头读到当前页末尾。
- 朗读过程中，当前语句应在 WPS 文档中同步选中。
- 朗读过程中，WPS 当前选中文本应与当前朗读文本一致。
- 朗读过程中，WPS 当前显示页应与当前朗读句子所在页一致；跨页长句按句首所在页判断。
- 朗读到表格时，空单元格应直接跳过，不朗读“空白内容”。
- 文档内图片、嵌入对象等非文本元素应直接跳过，不朗读、不停顿、不选中图片对象。
- 停止朗读后，不继续播放后续句子。
- 点击“停止朗读”后，应尽快中断当前正在播放的语句，不等待当前句自然结束。
- 启动提示显示“朗读服务正在启动，请耐心等待...”。
- 启动提示不固定倒计时关闭，进入实际播放后自动关闭。
- 启动提示、状态检查、关于朗读等弹窗关闭后，应交回 WPS 原生窗口焦点机制处理；加载项不得主动抢占前台、恢复窗口或最小化 WPS。
- Windows 11 文字缩放 200% 时，安装界面内容不应重叠，顶部宣传图应按比例完整显示。
- 低性能机器上仍可点击“停止朗读”结束任务。
- 0.75x、1x、1.2x、1.5x 四档语速均需验证可进入播放状态。
- 目录、短句和连续短段落朗读时，下一句应尽量无明显等待；如目标机器性能较低，应以“后台预合成仍在进行、界面可停止、不会报错”为最低验收要求。

## 功能按钮验收

- “开始朗读”：未朗读时可用；点击后弹出启动提示，进入朗读后按钮置灰。
- “停止朗读”：未朗读时置灰；朗读中可用；点击后立即停止朗读，并关闭启动提示。
- “朗读方式”：停止状态可切换“连页朗读”和“当页朗读”；朗读中置灰。
- “朗读语速”：停止状态可切换 0.75x、1x、1.2x、1.5x；朗读中置灰；默认 1.2x。
- “状态检查”：停止状态可打开服务状态弹窗；朗读中置灰。
- “关于朗读”：停止状态可打开关于弹窗；朗读中置灰。
- 所有按钮图标应正常显示，不出现问号、空白或明显缩小。

## 中英文数字验收

示例文本：

    这是 WPS 2026 read aloud test，版本是 v1.1.19。

预期结果：

- 中文正常朗读。
- 英文和数字不被跳过。
- 普通英文和数字按单字符中文读法朗读。
- “WPS”读作“达不溜屁挨思”，“Office”和“office”读作“凹斐思”。
- “10%”读作“百分之十”；加、减、乘、除、等于、大于等于、小于等于等数学符号按中文名称朗读。
- 逗号、顿号、分号、冒号等语义标点处有自然停顿。
- 双引号、单引号、书名号、括号等成对符号不额外增加停顿。

## 弹窗验收

- 弹窗不出现黄色三角叹号图标。
- 状态检查显示服务版本、语音引擎、自检结果、播放器和探测时间。
- 关于朗读标题为“WPS 文档朗读助手”。
- 关于朗读信息不重叠，默认大小下尽量不出现滚动条。
- 关于朗读中的说明文件可在同一弹窗内打开，并可返回关于页。
- 关闭弹窗后 WPS 不应异常最小化。

## 异常验收

- 连续快速点击“开始朗读”不会产生多段音频同时播放。
- 朗读中关闭 WPS 后，服务端不应残留长期运行的合成或播放进程。
- 网络不可用时仍可离线朗读。
- 服务只访问 127.0.0.1:19860。

