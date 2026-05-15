# 第三方组件声明

软件包：wps-read-aloud-zhangjingyao
开发者：zhangjingyao
发布时间：20260515

本软件包内置第三方离线语音运行组件，便于在纯净银河麒麟 V10 ARM64 系统上安装后直接使用。

## 内置组件

| 组件 | 包内路径 | 许可证 | 来源 |
| --- | --- | --- | --- |
| Sherpa-onnx | `/opt/wps-read-aloud/engines/sherpa-onnx/*` | Apache License 2.0 | https://github.com/k2-fsa/sherpa-onnx |
| ONNX Runtime | `/opt/wps-read-aloud/engines/sherpa-onnx/lib/libonnxruntime*` | MIT | https://github.com/microsoft/onnxruntime |
| 中文 Matcha 模型 | `/opt/wps-read-aloud/voices/sherpa/matcha-icefall-zh-baker/*` | 见上游模型发布说明 | https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models |
| 英文 Matcha 模型 | `/opt/wps-read-aloud/voices/sherpa/matcha-icefall-en_US-ljspeech/*` | 见上游模型发布说明 | https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models |
| Vocos 声码器模型 | `/opt/wps-read-aloud/voices/sherpa/vocos-22khz-univ.onnx` | 见上游模型发布说明 | https://github.com/k2-fsa/sherpa-onnx/releases/tag/vocoder-models |

## 隔离说明

第三方动态库不会安装到 `/usr/lib`、`/lib` 等系统公共库目录。服务进程只在启动 Sherpa-onnx 子进程时，为该子进程设置局部环境变量：

- `LD_LIBRARY_PATH`
- `ESPEAK_DATA_PATH`

这样可以避免覆盖或影响 WPS 以及其他软件使用的系统库。
