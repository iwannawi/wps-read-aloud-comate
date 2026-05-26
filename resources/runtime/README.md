# 多目标运行时资源

本目录存放不同 CPU 架构和操作系统的原生语音引擎文件。源码、WPS 加载项前端和语音模型共用，只有原生二进制和动态库按目标区分。

## 目录约定

    resources/runtime/windows-x86/sherpa-onnx/
    resources/runtime/linux-amd64/sherpa-onnx/
    resources/runtime/linux-arm64/sherpa-onnx/

每个 sherpa-onnx 目录至少需要包含：

- sherpa-onnx-offline-tts，或 x86/x64 Windows 10/11 下的 sherpa-onnx-offline-tts.exe
- lib 目录或同级依赖库

正式构建只从本目录读取对应 CPU 架构和操作系统的运行时资源。旧版 engines 目录不作为正式安装包输入，避免把目标环境不需要的库和废弃语音引擎带入安装包。

## 打包规则

- x64 银河麒麟系统和 x64 UOS系统安装包共用 linux-amd64 运行时资源。
- ARM64 银河麒麟系统和 ARM64 UOS系统安装包共用 linux-arm64 运行时资源。
- x86/x64 Windows 10/11 exe 安装程序使用 windows-x86 运行时资源；当前本地朗读服务不注入 WPS 进程，可服务 32 位和 64 位 WPS。
- 语音模型目录 voices/sherpa 不按系统区分。

运行时资源体积较大，默认不进入普通 Git 提交。正式发布时应通过受控制品库或 GitHub Release 附件保存。
