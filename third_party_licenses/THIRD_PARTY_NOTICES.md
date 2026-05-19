# 第三方组件声明

软件名称：WPS 文档朗读助手
软件包：wps-read-aloud-comate
开发者：Zhang Jingyao
发布时间：20260519

本软件包内置离线语音运行组件，用于在 x86/x64 Windows、银河麒麟 x64/ARM64、UOS x64/ARM64 等目标环境上提供本地文档朗读能力。第三方组件仅安装在本项目专用安装目录下，不写入系统公共库目录。

## 组件清单

| 组件 | 包内路径 | 许可证或状态 | 来源 |
| --- | --- | --- | --- |
| Sherpa-onnx | “engines/sherpa-onnx” | Apache License 2.0 | https://github.com/k2-fsa/sherpa-onnx |
| ONNX Runtime | “engines/sherpa-onnx” 中的 ONNX Runtime 动态库 | MIT License | https://github.com/microsoft/onnxruntime |
| 中文 VITS 模型 “vits-zh-hf-fanchen-C” | “voices/sherpa/vits-zh-hf-fanchen-C” | 上游模型权重未声明明确许可证；正式分发或商用前需单独确认授权，或替换为许可明确的模型 | https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models |

## 许可结论

- Sherpa-onnx 采用 Apache License 2.0。该许可证允许组织或个人在满足保留许可证文本、版权声明和 NOTICE 要求的前提下使用、复制、修改和分发。
- ONNX Runtime 采用 MIT License。该许可证允许组织或个人在保留版权声明和许可文本的前提下使用、复制、修改、合并、发布、分发、再许可和销售软件副本。
- “vits-zh-hf-fanchen-C” 模型由 Sherpa-onnx 官方文档列为预训练 TTS 模型，并说明其转换自 Hugging Face Space “lkz99/tts_model” 下的中文模型资源；但当前公开仓库未提供明确模型许可证和完整模型卡。因此，本项目不对该模型权重的正式分发或商用授权作出确认。

## 隔离说明

第三方动态库不会安装到 “/usr/lib”、“/lib” 等系统公共库目录。服务进程只在启动 Sherpa-onnx 子进程时，为该子进程设置局部环境变量：

- “LD_LIBRARY_PATH”

该设计用于避免覆盖或影响 WPS 以及其他软件使用的系统库。

## 使用提示

如果安装包用于组织或个人的正式对外分发、商业项目或长期生产环境，请在发布前完成 “vits-zh-hf-fanchen-C” 模型权重的来源、训练数据、再分发和商用授权确认。无法取得明确授权时，应替换为许可证明确允许相应使用场景的中文语音模型。
