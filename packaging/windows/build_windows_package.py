#!/usr/bin/env python3
import json
import os
import shutil
import subprocess
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
VERSION = os.environ.get("VERSION", "1.1.17")
RELEASE_DATE = os.environ.get("RELEASE_DATE", "20260525")
WINDOWS_ARCH = os.environ.get("WINDOWS_ARCH", "386")
ARCH_LABEL = "x86" if WINDOWS_ARCH in {"386", "x86"} else WINDOWS_ARCH
PKG_NAME = "wps-read-aloud-comate"
OUT = ROOT / "dist"
BUILD = ROOT / "build" / "windows" / f"{PKG_NAME}_{VERSION}_windows_{ARCH_LABEL}"
RUNTIME = ROOT / "resources" / "runtime" / f"windows-{ARCH_LABEL}"
VOICE = ROOT / "voices" / "sherpa"
ADDIN = ROOT / "addin"
EXE = OUT / f"{PKG_NAME}_{VERSION}_windows.exe"
INSTALLER_SRC = ROOT / "packaging" / "windows" / "installer"
INSTALLER_ASSETS = ROOT / "packaging" / "windows" / "assets"


def require(path: Path, message: str) -> None:
    if not path.exists():
        raise SystemExit(message)


def copytree(src: Path, dst: Path) -> None:
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(src, dst)


def write_version_json(target: Path) -> None:
    target.write_text(
        json.dumps(
            {
                "name": "WPS 文档朗读助手",
                "package": PKG_NAME,
                "version": VERSION,
                "release_date": RELEASE_DATE,
                "system": "windows",
                "architecture": ARCH_LABEL,
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )


def write_windows_config(target: Path) -> None:
    target.write_text(
        "\n".join(
            [
                'listen: "127.0.0.1:19860"',
                "sherpa:",
                '  bin: "engines/sherpa-onnx/sherpa-onnx-offline-tts.exe"',
                "  num_threads: 4",
                "  target_sample_rate: 16000",
                '  vits_model: "voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx"',
                '  vits_lexicon: "voices/sherpa/vits-zh-hf-fanchen-C/lexicon.txt"',
                '  vits_tokens: "voices/sherpa/vits-zh-hf-fanchen-C/tokens.txt"',
                '  vits_rule_fsts: "voices/sherpa/vits-zh-hf-fanchen-C/phone.fst,voices/sherpa/vits-zh-hf-fanchen-C/date.fst,voices/sherpa/vits-zh-hf-fanchen-C/number.fst,voices/sherpa/vits-zh-hf-fanchen-C/new_heteronym.fst"',
                "  vits_speaker_id: 14",
                "",
            ]
        ),
        encoding="utf-8",
        newline="\r\n",
    )


def zip_dir(src: Path, dst: Path) -> None:
    if dst.exists():
        dst.unlink()
    with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zf:
        for path in sorted(src.rglob("*")):
            if path.is_file():
                if path.resolve() == dst.resolve():
                    continue
                zf.write(path, path.relative_to(src))


def go_exe() -> Path:
    bundled = ROOT / "tools" / "go" / "bin" / ("go.exe" if os.name == "nt" else "go")
    if bundled.exists():
        return bundled
    found = shutil.which("go")
    if found:
        return Path(found)
    raise SystemExit("missing Go toolchain; cannot build Windows exe installer")


def build_installer_exe(payload_zip: Path) -> None:
    work = BUILD / "installer-src"
    if work.exists():
        shutil.rmtree(work)
    shutil.copytree(INSTALLER_SRC, work)
    env = os.environ.copy()
    env["GOOS"] = "windows"
    env["GOARCH"] = "386" if ARCH_LABEL == "x86" else WINDOWS_ARCH
    env["CGO_ENABLED"] = "0"
    env.setdefault("GOCACHE", str(ROOT / "build" / "gocache"))
    if EXE.exists():
        EXE.unlink()
    subprocess.run(
        [str(go_exe()), "build", "-buildvcs=false", "-ldflags", "-H=windowsgui", "-o", str(EXE), "."],
        cwd=work,
        env=env,
        check=True,
    )
    with EXE.open("ab") as fh:
        fh.write(b"WPS_READ_ALOUD_COMATE_PAYLOAD_ZIP_V1\n")
        fh.write(payload_zip.read_bytes())


def main() -> None:
    require(
        RUNTIME / "sherpa-onnx",
        f"missing Windows runtime: {RUNTIME / 'sherpa-onnx'}",
    )
    require(
        OUT / f"wps-tts-daemon-windows-{ARCH_LABEL}.exe",
        f"missing Windows daemon: {OUT / f'wps-tts-daemon-windows-{ARCH_LABEL}.exe'}",
    )
    require(VOICE, "missing shared voice model directory: voices/sherpa")

    if BUILD.exists():
        shutil.rmtree(BUILD)
    OUT.mkdir(parents=True, exist_ok=True)
    app = BUILD / "app"
    app.mkdir(parents=True, exist_ok=True)

    copytree(ADDIN, app / "addin")
    copytree(RUNTIME / "sherpa-onnx", app / "engines" / "sherpa-onnx")
    copytree(VOICE, app / "voices" / "sherpa")
    copytree(ROOT / "third_party_licenses", app / "third_party_licenses")
    for doc in ["RELEASE_NOTES.md", "ACCEPTANCE_TEST.md", "SOURCE_OFFER.md"]:
        shutil.copy2(ROOT / doc, app / doc)
    (app / "daemon").mkdir(parents=True, exist_ok=True)
    shutil.copy2(OUT / f"wps-tts-daemon-windows-{ARCH_LABEL}.exe", app / "daemon" / "wps-tts-daemon.exe")
    shutil.copy2(ROOT / "packaging" / "windows" / "install.ps1", BUILD / "install.ps1")
    shutil.copy2(ROOT / "packaging" / "windows" / "install-ui.ps1", BUILD / "install-ui.ps1")
    shutil.copy2(ROOT / "packaging" / "windows" / "uninstall.ps1", BUILD / "uninstall.ps1")
    if INSTALLER_ASSETS.is_dir():
        copytree(INSTALLER_ASSETS, BUILD / "installer-assets")
    write_version_json(app / "version.json")
    write_windows_config(app / "config.yaml")
    payload_zip = BUILD / "payload.zip"
    zip_dir(BUILD, payload_zip)
    build_installer_exe(payload_zip)
    print(f"created {EXE}")


if __name__ == "__main__":
    main()
