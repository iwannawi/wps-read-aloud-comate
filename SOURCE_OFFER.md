# 第三方源码获取说明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
版本：1.1.3
开发者：Zhang Jingyao
发布时间：20260521

本软件内置第三方开源组件，用于在目标环境中离线运行。

## 目标环境

| CPU 架构 + 操作系统 | 安装路径 |
| --- | --- |
| x86/x64 Windows 10/11 | 用户选择的程序目录 |
| x64 银河麒麟 V10 及以上 | /opt/wps-read-aloud-comate |
| ARM64 银河麒麟 V10 及以上 | /opt/wps-read-aloud-comate |
| x64 UOS V20 | /opt/apps/cn.wps-read-aloud-comate/files |
| ARM64 UOS V20 | /opt/apps/cn.wps-read-aloud-comate/files |

## Sherpa-onnx

- 许可证：Apache License 2.0
- 源码地址：https://github.com/k2-fsa/sherpa-onnx
- 包内路径：engines/sherpa-onnx

## ONNX Runtime

- 许可证：MIT
- 源码地址：https://github.com/microsoft/onnxruntime
- 包内路径：engines/sherpa-onnx/lib

## Sherpa-onnx VITS 语音模型

- 中文模型：vits-zh-hf-fanchen-C
- 上游模型发布页：https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models
- Hugging Face 镜像页：https://huggingface.co/csukuangfj/vits-zh-hf-fanchen-C
- 包内路径：voices/sherpa/vits-zh-hf-fanchen-C

神经语音模型的再分发可能涉及合规要求。正式对外分发前，建议复核模型来源、训练数据和再分发许可，必要时取得授权或替换为许可更明确的模型。
