#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

required=(
  "dist/wps-tts-daemon"
  "engines/sherpa-onnx/sherpa-onnx-offline-tts"
  "engines/sherpa-onnx/lib"
  "voices/sherpa/matcha-icefall-zh-baker/model-steps-3.onnx"
  "voices/sherpa/matcha-icefall-zh-baker/lexicon.txt"
  "voices/sherpa/matcha-icefall-zh-baker/tokens.txt"
  "voices/sherpa/matcha-icefall-en_US-ljspeech/model-steps-3.onnx"
  "voices/sherpa/matcha-icefall-en_US-ljspeech/tokens.txt"
  "voices/sherpa/matcha-icefall-en_US-ljspeech/espeak-ng-data"
  "voices/sherpa/vocos-22khz-univ.onnx"
)

missing=0
for item in "${required[@]}"; do
  if [[ ! -e "${ROOT_DIR}/${item}" ]]; then
    echo "MISSING ${item}"
    missing=1
  else
    echo "OK      ${item}"
  fi
done

if [[ "${missing}" -ne 0 ]]; then
  echo "offline payload is incomplete; refusing to build a clean-system package." >&2
  exit 1
fi

echo "offline payload check passed."
