# Git 与 GitHub 版本管理规范

## 管理范围

源码仓库只提交加载项源码、服务端源码、打包脚本、配置模板、说明文档和许可证文件。

不直接提交普通 Git 的内容：

- `dist/` 发布安装包和构建出的二进制文件
- `engines/` 离线语音引擎
- `voices/` 语音模型
- `tools/` 本地工具链
- `build/`、`.gocache*`、`downloads/` 等缓存目录

这些大文件建议放到 GitHub Release 附件、企业制品库或受控内网文件服务器。

## 分支策略

- `main`：稳定交付分支，只合入已验证版本。
- `develop`：日常开发分支。
- `feature/<功能名>`：单个功能或修复分支。
- `hotfix/<问题名>`：生产问题紧急修复分支。

## 版本号与标签

版本号遵循 `主版本.次版本.修订号`，例如 `1.0.1`。

发布标签格式：

```bash
v1.0.1-20260515
```

标签应指向已完成验收的提交。发布 `.deb` 时，需要同时记录 SHA256。

## 推荐提交流程

```bash
git status
git add <变更文件>
git commit -m "简要说明本次变更"
git tag v1.0.1-20260515
```

## GitHub 同步流程

首次同步到 GitHub：

```bash
git remote add origin git@github.com:<owner>/<repo>.git
git push -u origin main
git push origin v1.0.1-20260515
```

如果使用 HTTPS：

```bash
git remote add origin https://github.com/<owner>/<repo>.git
git push -u origin main
git push origin v1.0.1-20260515
```

## 发布附件

最终 `.deb` 不进普通源码提交，建议上传到 GitHub Release，并在发布说明中填写：

- 安装包名称
- 版本号
- 架构
- SHA256
- 主要变更
- 已知限制

当前交付包名示例：

```text
wps-read-aloud-zhangjingyao_1.0.1_arm64.deb
```

## Codex 自动执行

本项目后续由 Codex 自动执行本地 Git 与 GitHub 版本管理，用户不需要手动运行提交、推送、打标签或构建命令。

详细规则见：

```text
docs/CODEX_AUTOMATION.md
```
