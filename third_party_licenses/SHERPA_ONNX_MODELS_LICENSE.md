# Sherpa-onnx 语音模型许可说明

本安装包内置以下 Sherpa-onnx 官方发布的离线 TTS 模型文件：

- “vits-zh-hf-fanchen-C”

模型来源：

- https://github.com/k2-fsa/sherpa-onnx/releases/tag/tts-models
- https://huggingface.co/csukuangfj/vits-zh-hf-fanchen-C

Sherpa-onnx 主项目采用 Apache License 2.0。需要注意的是，语音模型权重、训练数据集和转换脚本可能具有独立的版权或许可条件，不能直接等同于 Sherpa-onnx 主项目许可证。

## 已核实信息

- Sherpa-onnx 官方文档将 “csukuangfj/vits-zh-hf-fanchen-C” 列为中文 VITS 预训练模型。
- Sherpa-onnx 官方文档说明该模型转换自 Hugging Face Space “lkz99/tts_model” 下的中文模型资源。
- 当前公开的 “csukuangfj/vits-zh-hf-fanchen-C” 仓库未提供明确模型许可证和完整模型卡。

## 许可结论

由于模型权重未声明明确许可证，本项目不能确认 “vits-zh-hf-fanchen-C” 可直接用于正式分发或商用场景。包含该模型的安装包可用于技术验证、内部功能测试和音质评估。

如果组织或个人计划将该安装包用于正式对外分发、商业项目或长期生产环境，应先完成以下事项：

1. 确认模型权重来源和权利归属。
2. 确认训练数据和声音来源是否允许相应使用场景。
3. 确认模型权重是否允许随安装包再分发。
4. 无法取得明确授权时，替换为许可证明确允许相应使用场景的中文语音模型。

本项目未修改上述模型文件，仅作为离线运行资源随安装包分发到本项目专用语音模型目录。银河麒麟包路径为 “/opt/wps-read-aloud-comate/voices/sherpa”，UOS 包路径为 “/opt/apps/cn.wps-read-aloud-comate/files/voices/sherpa”，Windows 包路径为安装目录下的 “voices\sherpa”。
