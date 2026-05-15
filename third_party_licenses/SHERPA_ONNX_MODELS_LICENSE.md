# Sherpa-onnx 语音模型许可说明

本安装包内置以下 Sherpa-onnx 官方发布的离线 TTS 模型文件：

- `matcha-icefall-zh-baker`
- `matcha-icefall-en_US-ljspeech`
- `vocos-22khz-univ.onnx`

来源：

- https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models
- https://github.com/k2-fsa/sherpa-onnx/releases/tag/vocoder-models

Sherpa-onnx 项目采用 Apache License 2.0。上述模型随 Sherpa-onnx 官方 release 分发，企业交付时应同时保留本说明、上游来源链接，以及安装包内 `SHERPA_ONNX_LICENSE.md`、`ONNXRUNTIME_LICENSE.txt` 等许可文件。

本项目未修改上述模型文件，仅作为离线运行资源随安装包一起分发到 `/opt/wps-read-aloud/voices/sherpa`。
