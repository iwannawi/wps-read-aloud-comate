# 第三方源码获取说明

软件包：wps-read-aloud-zhangjingyao
版本：1.0.14
开发者：zhangjingyao
发布时间：20260516

本软件包内置第三方开源组件，以便在纯净银河麒麟 V10 ARM64 系统上离线运行。第三方组件安装在 `/opt/wps-read-aloud`，许可证和声明文件安装在 `/usr/share/doc/wps-read-aloud-zhangjingyao`。

## Sherpa-onnx

- 许可证：Apache License 2.0
- 源码地址：https://github.com/k2-fsa/sherpa-onnx
- 安装路径：`/opt/wps-read-aloud/engines/sherpa-onnx`

## ONNX Runtime

- 许可证：MIT
- 源码地址：https://github.com/microsoft/onnxruntime
- 安装路径：`/opt/wps-read-aloud/engines/sherpa-onnx/lib/libonnxruntime*`

## Sherpa-onnx VITS 语音模型

- 中文模型：`vits-zh-hf-fanchen-C`
- 上游模型发布页：https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models
- Hugging Face 镜像页：https://huggingface.co/csukuangfj/vits-zh-hf-fanchen-C

包内文件：

- `/opt/wps-read-aloud/voices/sherpa/vits-zh-hf-fanchen-C/`

神经语音模型的再分发可能涉及单位内部合规要求，正式对外分发前建议进行法务或合规审核。特别是当前中文模型 `vits-zh-hf-fanchen-C` 的 Hugging Face 仓库未提供完整模型卡和明确许可证；如用于正式政企或商业交付，应先完成授权确认，或替换为许可明确的中文模型。
