# 第三方源码获取说明

软件包：wps-read-aloud-zhangjingyao
版本：1.0.8
开发者：zhangjingyao
发布时间：20260515

本软件包内置第三方开源组件，以便在纯净银河麒麟 V10 ARM64 系统上离线运行。第三方组件安装在 `/opt/wps-read-aloud`，许可证和声明文件安装在 `/usr/share/doc/wps-read-aloud-zhangjingyao`。

## Piper

- 许可证：MIT
- 源码地址：https://github.com/rhasspy/piper
- 安装路径：`/opt/wps-read-aloud/engines/piper/piper`

## ONNX Runtime

- 许可证：MIT
- 源码地址：https://github.com/microsoft/onnxruntime
- 安装路径：`/opt/wps-read-aloud/engines/piper/lib/libonnxruntime*`

## eSpeak NG

- 许可证：GPL-3.0-or-later
- 源码地址：https://github.com/espeak-ng/espeak-ng
- 安装路径：`/opt/wps-read-aloud/engines/espeak-ng`

为满足 GPL 合规要求，向外部分发本安装包时，应同时提供与包内 eSpeak NG 二进制对应的源码获取方式。可以提供：

- 构建该二进制所使用的 eSpeak NG 源码压缩包；或
- 书面源码提供说明，以及稳定的内部或外部下载地址。

## Piper 中文语音模型

- 上游模型集合：https://huggingface.co/rhasspy/piper-voices
- 上游模型卡声明许可证：MIT
- 包内文件：
  - `/opt/wps-read-aloud/voices/zh_CN.onnx`
  - `/opt/wps-read-aloud/voices/zh_CN.onnx.json`

神经语音模型的再分发可能涉及单位内部合规要求，正式对外分发前建议进行法务或合规审核。
