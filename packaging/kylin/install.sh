#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_DIR="/opt/wps-read-aloud"
CONFIG_DIR="/etc/wps-read-aloud"
SERVICE_DIR="${HOME}/.config/systemd/user"

if [[ "$(uname -m)" != "aarch64" && "$(uname -m)" != "arm64" ]]; then
  echo "提示：当前机器不是 ARM64，仍会继续安装。"
fi

if [[ ! -x "${ROOT_DIR}/dist/wps-tts-daemon" ]]; then
  echo "缺少 ${ROOT_DIR}/dist/wps-tts-daemon，请先编译 Go 服务。"
  exit 1
fi

sudo mkdir -p "${APP_DIR}/daemon" "${APP_DIR}/addin" "${APP_DIR}/voices" "${APP_DIR}/engines/sherpa-onnx" "${CONFIG_DIR}"
sudo cp "${ROOT_DIR}/dist/wps-tts-daemon" "${APP_DIR}/daemon/wps-tts-daemon"
sudo cp -r "${ROOT_DIR}/addin/." "${APP_DIR}/addin/"
sudo chmod +x "${APP_DIR}/daemon/wps-tts-daemon"

if compgen -G "${ROOT_DIR}/voices/*" >/dev/null; then
  sudo cp -r "${ROOT_DIR}/voices/." "${APP_DIR}/voices/"
fi

if [[ -d "${ROOT_DIR}/engines" ]]; then
  sudo cp -r "${ROOT_DIR}/engines/." "${APP_DIR}/engines/"
fi

if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
  sudo cp "${ROOT_DIR}/daemon/config.example.yaml" "${CONFIG_DIR}/config.yaml"
fi

mkdir -p "${SERVICE_DIR}"
cp "${ROOT_DIR}/packaging/kylin/wps-tts.service" "${SERVICE_DIR}/wps-tts.service"
systemctl --user daemon-reload
systemctl --user enable --now wps-tts.service

echo "安装完成。"
echo "服务检查：curl http://127.0.0.1:19860/health"
echo "加载项目录：${APP_DIR}/addin"
