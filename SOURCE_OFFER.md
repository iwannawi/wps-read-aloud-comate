# 第三方源码获取说明

软件包：wps-read-aloud-zhangjingyao
版本：1.0.10
开发者：zhangjingyao
发布时间：20260515

本软件包内置第三方开源组件，以便在纯净银河麒麟 V10 ARM64 系统上离线运行。第三方组件安装在 `/opt/wps-read-aloud`，许可证和声明文件安装在 `/usr/share/doc/wps-read-aloud-zhangjingyao`。

## Sherpa-onnx

- 许可证：Apache License 2.0
- 源码地址：https://github.com/k2-fsa/sherpa-onnx
- 安装路径：`/opt/wps-read-aloud/engines/sherpa-onnx`

## ONNX Runtime

- 许可证：MIT
- 源码地址：https://github.com/microsoft/onnxruntime
- 安装路径：`/opt/wps-read-aloud/engines/sherpa-onnx/lib/libonnxruntime*`

## Sherpa-onnx Matcha 语音模型

- 中文模型：`matcha-icefall-zh-baker`
- 英文模型：`matcha-icefall-en_US-ljspeech`
- 声码器模型：`vocos-22khz-univ.onnx`
- 上游模型发布页：https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models
- 声码器发布页：https://github.com/k2-fsa/sherpa-onnx/releases/tag/vocoder-models

包内文件：

- `/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/`
- `/opt/wps-read-aloud/voices/sherpa/matcha-icefall-en_US-ljspeech/`
- `/opt/wps-read-aloud/voices/sherpa/vocos-22khz-univ.onnx`

神经语音模型的再分发可能涉及单位内部合规要求，正式对外分发前建议进行法务或合规审核。
