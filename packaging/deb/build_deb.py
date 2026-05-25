#!/usr/bin/env python3
import gzip
import io
import json
import os
import shutil
import tarfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
BASE_PKG_NAME = "wps-read-aloud-comate"
UOS_APP_ID = "cn.wps-read-aloud-comate"
SERVICE_NAME = "wps-read-aloud-comate.service"
LEGACY_SERVICE_NAME = "wps-tts.service"
REGISTER_BIN_NAME = "wps-read-aloud-comate-register"
VERSION = os.environ.get("VERSION", "1.1.17")
RELEASE_DATE = os.environ.get("RELEASE_DATE", "20260525")
ARCH = os.environ.get("ARCH", "arm64")
DISTRO = os.environ.get("DISTRO", "kylin").lower()
DISTRO_LABELS = {
    "kylin": "银河麒麟 V10 及以上",
    "uos": "UOS V20",
}
ARCH_LABELS = {
    "amd64": "x64",
    "arm64": "ARM64",
}
if DISTRO not in DISTRO_LABELS:
    raise SystemExit(f"unsupported DISTRO: {DISTRO}; expected one of {', '.join(sorted(DISTRO_LABELS))}")
if ARCH not in ARCH_LABELS:
    raise SystemExit(f"unsupported ARCH: {ARCH}; expected one of {', '.join(sorted(ARCH_LABELS))}")
PLATFORM_LABEL = f"{ARCH_LABELS[ARCH]} {DISTRO_LABELS[DISTRO]}"
OUT = ROOT / "dist"
PKG_NAME = UOS_APP_ID if DISTRO == "uos" else BASE_PKG_NAME
ARTIFACT_NAME = UOS_APP_ID if DISTRO == "uos" else BASE_PKG_NAME
APP_ROOT_REL = f"opt/apps/{UOS_APP_ID}/files" if DISTRO == "uos" else "opt/wps-read-aloud-comate"
CONFIG_REL = f"{APP_ROOT_REL}/config.yaml" if DISTRO == "uos" else "etc/wps-read-aloud-comate/config.yaml"
DOC_REL = f"{APP_ROOT_REL}/doc" if DISTRO == "uos" else f"usr/share/doc/{BASE_PKG_NAME}"
BUILD = ROOT / "build" / "deb" / f"{PKG_NAME}_{VERSION}_{DISTRO}_{ARCH}"
DATA = BUILD / "data"
DEBIAN = BUILD / "DEBIAN"
DEB = OUT / f"{ARTIFACT_NAME}_{VERSION}_{ARCH}.deb"
ADDIN = ROOT / "addin"
EMBEDDED_WEB = ROOT / "daemon" / "cmd" / "wps-tts-daemon" / "web"
RUNTIME_ROOT = ROOT / "resources" / "runtime"


REQUIRED = [
    "voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx",
    "voices/sherpa/vits-zh-hf-fanchen-C/lexicon.txt",
    "voices/sherpa/vits-zh-hf-fanchen-C/tokens.txt",
    "voices/sherpa/vits-zh-hf-fanchen-C/phone.fst",
    "voices/sherpa/vits-zh-hf-fanchen-C/date.fst",
    "voices/sherpa/vits-zh-hf-fanchen-C/number.fst",
    "voices/sherpa/vits-zh-hf-fanchen-C/new_heteronym.fst",
    "third_party_licenses/THIRD_PARTY_NOTICES.md",
    "third_party_licenses/SHERPA_ONNX_LICENSE.md",
    "third_party_licenses/SHERPA_ONNX_MODELS_LICENSE.md",
    "third_party_licenses/ONNXRUNTIME_LICENSE.txt",
    "RELEASE_NOTES.md",
    "ACCEPTANCE_TEST.md",
    "SOURCE_OFFER.md",
    "docs/MULTI_PLATFORM_PACKAGING.md",
]

EXECUTABLE_SUFFIXES = {
    f"{APP_ROOT_REL}/daemon/wps-tts-daemon",
    f"{APP_ROOT_REL}/engines/sherpa-onnx/sherpa-onnx-offline-tts",
    f"usr/bin/{REGISTER_BIN_NAME}",
}

DOC_FILES = [
    "THIRD_PARTY_NOTICES.md",
    "SHERPA_ONNX_LICENSE.md",
    "SHERPA_ONNX_MODELS_LICENSE.md",
    "ONNXRUNTIME_LICENSE.txt",
]

PROJECT_DOC_FILES = [
    "RELEASE_NOTES.md",
    "ACCEPTANCE_TEST.md",
    "SOURCE_OFFER.md",
    "docs/MULTI_PLATFORM_PACKAGING.md",
]

DUPLICATE_LIBRARY_LINKS = {}

EXCLUDED_PACKAGE_FILES = {
    f"{APP_ROOT_REL}/engines/.gitkeep",
    f"{APP_ROOT_REL}/engines/README.md",
    f"{APP_ROOT_REL}/voices/.gitkeep",
}


def require(path: str) -> None:
    full = ROOT / path
    if not full.exists():
        raise SystemExit(f"missing required file: {path}")


def extract_ar_member(deb_path: Path, member_name: str) -> bytes:
    data = deb_path.read_bytes()
    if not data.startswith(b"!<arch>\n"):
        raise ValueError(f"not a deb/ar file: {deb_path}")
    pos = 8
    while pos + 60 <= len(data):
        header = data[pos : pos + 60]
        name = header[:16].decode("ascii").strip()
        size = int(header[48:58].decode("ascii").strip())
        pos += 60
        payload = data[pos : pos + size]
        pos += size + (size % 2)
        if name == member_name:
            return payload
    raise KeyError(member_name)


def extract_daemon_from_deb(deb_path: Path, target: Path) -> bool:
    try:
        data_tar = extract_ar_member(deb_path, "data.tar.gz")
        with tarfile.open(fileobj=io.BytesIO(data_tar), mode="r:gz") as tar:
            member = None
            for name in (
                f"{APP_ROOT_REL}/daemon/wps-tts-daemon",
                "opt/wps-read-aloud/daemon/wps-tts-daemon",
                f"opt/apps/{UOS_APP_ID}/files/daemon/wps-tts-daemon",
            ):
                try:
                    member = tar.getmember(name)
                    break
                except KeyError:
                    continue
            if member is None:
                return False
            source = tar.extractfile(member)
            if source is None:
                return False
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(source.read())
            target.chmod(0o755)
            return True
    except Exception:
        return False


def resolve_daemon_binary() -> Path:
    daemon = OUT / f"wps-tts-daemon-linux-{ARCH}"
    if daemon.is_file():
        return daemon
    legacy_daemon = OUT / "wps-tts-daemon"
    if ARCH == "arm64" and legacy_daemon.is_file():
        return legacy_daemon
    candidates = sorted(
        list(OUT.glob(f"{ARTIFACT_NAME}_*_{DISTRO}_{ARCH}.deb"))
        + list(OUT.glob(f"{BASE_PKG_NAME}_*_{DISTRO}_{ARCH}.deb"))
        + list(OUT.glob(f"{UOS_APP_ID}_*_{DISTRO}_{ARCH}.deb"))
        + list(OUT.glob(f"{ARTIFACT_NAME}_*_{ARCH}.deb"))
        + list(OUT.glob(f"{BASE_PKG_NAME}_*_{ARCH}.deb"))
        + list(OUT.glob(f"{UOS_APP_ID}_*_{ARCH}.deb")),
        key=lambda path: path.stat().st_mtime,
        reverse=True,
    )
    for candidate in candidates:
        if extract_daemon_from_deb(candidate, daemon):
            return daemon
    raise SystemExit(
        f"missing required file: dist/wps-tts-daemon-linux-{ARCH}; "
        "no previous package was available to reuse the daemon binary"
    )


def write_version_json(target: Path) -> None:
    info = {
        "name": "WPS 文档朗读助手",
        "package": PKG_NAME,
        "base_package": BASE_PKG_NAME,
        "version": VERSION,
        "release_date": RELEASE_DATE,
        "distro": DISTRO,
        "architecture": ARCH,
        "install_root": "/" + APP_ROOT_REL,
    }
    target.write_text(json.dumps(info, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_linux_config(target: Path) -> None:
    root = "/" + APP_ROOT_REL
    voice = f"{root}/voices/sherpa/vits-zh-hf-fanchen-C"
    target.write_text(
        "\n".join(
            [
                'listen: "127.0.0.1:19860"',
                "sherpa:",
                f'  bin: "{root}/engines/sherpa-onnx/sherpa-onnx-offline-tts"',
                "  num_threads: 2",
                "  target_sample_rate: 16000",
                f'  vits_model: "{voice}/vits-zh-hf-fanchen-C.onnx"',
                f'  vits_lexicon: "{voice}/lexicon.txt"',
                f'  vits_tokens: "{voice}/tokens.txt"',
                f'  vits_rule_fsts: "{voice}/phone.fst,{voice}/date.fst,{voice}/number.fst,{voice}/new_heteronym.fst"',
                "  vits_speaker_id: 14",
                "",
            ]
        ),
        encoding="utf-8",
        newline="\n",
    )


def write_systemd_service(target: Path) -> None:
    root = "/" + APP_ROOT_REL
    target.write_text(
        "\n".join(
            [
                "[Unit]",
                "Description=WPS Read Aloud Local TTS Service",
                "After=network.target sound.target",
                "",
                "[Service]",
                "Type=simple",
                f"Environment=WPS_READ_ALOUD_ROOT={root}",
                f"Environment=WPS_READ_ALOUD_DOC_DIR=/{DOC_REL}",
                f"ExecStart={root}/daemon/wps-tts-daemon -config /{CONFIG_REL}",
                "Restart=on-failure",
                "RestartSec=2",
                "",
                "[Install]",
                "WantedBy=multi-user.target",
                "",
            ]
        ),
        encoding="utf-8",
        newline="\n",
    )


def write_debian_script(script: str) -> None:
    src = ROOT / "packaging" / "deb" / script
    text = src.read_text(encoding="utf-8")
    replacements = {
        "@APP_ROOT@": "/" + APP_ROOT_REL,
        "@CONFIG_DIR@": "/" + str(Path(CONFIG_REL).parent).replace("\\", "/"),
        "@CONFIG_PATH@": "/" + CONFIG_REL,
        "@DOC_DIR@": "/" + DOC_REL,
        "@PACKAGE_NAME@": PKG_NAME,
        "@BASE_PACKAGE_NAME@": BASE_PKG_NAME,
        "@ADDIN_VERSION@": VERSION,
        "@SERVICE_NAME@": SERVICE_NAME,
        "@LEGACY_SERVICE_NAME@": LEGACY_SERVICE_NAME,
        "@REGISTER_BIN@": f"/usr/bin/{REGISTER_BIN_NAME}",
    }
    for key, value in replacements.items():
        text = text.replace(key, value)
    (DEBIAN / script).write_text(text, encoding="utf-8", newline="\n")


def linux_runtime_id() -> str:
    if ARCH == "amd64":
        return "linux-amd64"
    if ARCH == "arm64":
        return "linux-arm64"
    raise SystemExit(f"unsupported ARCH: {ARCH}; expected amd64 or arm64")


def sherpa_runtime_source() -> Path:
    source = RUNTIME_ROOT / linux_runtime_id() / "sherpa-onnx"
    if source.is_dir():
        return source
    raise SystemExit(
        f"missing runtime files: {source}. "
        "Place the matching sherpa-onnx binary and libraries under resources/runtime/<platform>/sherpa-onnx."
    )


def copytree_contents(src: Path, dst: Path) -> None:
    dst.mkdir(parents=True, exist_ok=True)
    for item in src.iterdir():
        target = dst / item.name
        if item.is_dir():
            shutil.copytree(item, target, dirs_exist_ok=True)
        else:
            shutil.copy2(item, target)


def verify_addin_web_synced() -> None:
    for src in sorted(ADDIN.rglob("*")):
        rel = src.relative_to(ADDIN)
        target = EMBEDDED_WEB / rel
        if src.is_dir():
            if not target.is_dir():
                raise SystemExit(f"embedded web assets are not synchronized: missing directory {target}")
            continue
        if not target.is_file():
            raise SystemExit(f"embedded web assets are not synchronized: missing file {target}")
        if src.read_bytes() != target.read_bytes():
            raise SystemExit(f"embedded web assets are not synchronized: {rel}")
    for target in sorted(EMBEDDED_WEB.rglob("*")):
        rel = target.relative_to(EMBEDDED_WEB)
        src = ADDIN / rel
        if not src.exists():
            raise SystemExit(f"embedded web assets contain extra file: {target}")


def normalize_control() -> None:
    src = ROOT / "packaging" / "deb" / "control"
    lines = src.read_text(encoding="utf-8").splitlines()
    out = []
    for line in lines:
        if line.startswith("Package:"):
            out.append(f"Package: {PKG_NAME}")
        elif line.startswith("Version:"):
            out.append(f"Version: {VERSION}")
        elif line.startswith("Architecture:"):
            out.append(f"Architecture: {ARCH}")
        elif line.startswith("Provides:"):
            out.append(f"Provides: {BASE_PKG_NAME}, wps-read-aloud")
        elif line.startswith("Conflicts:") or line.startswith("Replaces:"):
            continue
        elif line.startswith("Description:"):
            out.append(f"Description: WPS 文档朗读助手 for {PLATFORM_LABEL}")
        elif line.startswith(" Supports "):
            out.append(f" Supports {PLATFORM_LABEL}. Requires WPS Office 2019 or later; latest stable WPS Office for Linux is recommended.")
        else:
            out.append(line)
    (DEBIAN / "control").write_text("\n".join(out) + "\n", encoding="utf-8", newline="\n")


def normalize_tarinfo(info: tarfile.TarInfo) -> tarfile.TarInfo:
    info.uid = 0
    info.gid = 0
    info.uname = "root"
    info.gname = "root"
    info.mtime = 0
    return info


def tar_bytes(
    root: Path,
    names: list[str],
    gzip_output: bool,
    symlinks: dict[str, str] | None = None,
) -> bytes:
    raw = io.BytesIO()
    fileobj = gzip.GzipFile(fileobj=raw, mode="wb", mtime=0) if gzip_output else raw
    with tarfile.open(fileobj=fileobj, mode="w", format=tarfile.USTAR_FORMAT) as tar:
        for name in names:
            path = root / name
            arcname = name.replace("\\", "/")
            info = normalize_tarinfo(tar.gettarinfo(str(path), arcname=arcname))
            if info.isdir():
                info.mode = 0o755
                tar.addfile(info)
                continue
            rel = arcname.lstrip("./")
            info.mode = 0o755 if rel in EXECUTABLE_SUFFIXES or rel in {"preinst", "postinst", "prerm", "postrm"} else 0o644
            with path.open("rb") as fh:
                tar.addfile(info, fh)
        for link_name, target in sorted((symlinks or {}).items()):
            info = normalize_tarinfo(tarfile.TarInfo(link_name))
            info.type = tarfile.SYMTYPE
            info.linkname = target
            info.mode = 0o777
            tar.addfile(info)
    if gzip_output:
        fileobj.close()
    return raw.getvalue()


def all_data_names() -> list[str]:
    names: list[str] = []
    for path in sorted(DATA.rglob("*")):
        rel = path.relative_to(DATA).as_posix()
        if rel in EXCLUDED_PACKAGE_FILES:
            continue
        if rel.endswith("/.gitkeep") or "/__pycache__/" in rel or rel.endswith(".pyc") or "/._" in rel or rel.startswith("._"):
            continue
        names.append(rel)
    return names


def replace_duplicate_libraries_with_links() -> None:
    for link_name in DUPLICATE_LIBRARY_LINKS:
        duplicate = DATA / link_name
        if duplicate.exists() and not duplicate.is_dir():
            duplicate.unlink()


def ar_member(name: str, data: bytes) -> bytes:
    header = f"{name:<16}{0:<12}{0:<6}{0:<6}{0o100644:<8}{len(data):<10}`\n"
    encoded = header.encode("ascii")
    if len(encoded) != 60:
        raise ValueError(f"invalid ar header for {name}")
    pad = b"\n" if len(data) % 2 else b""
    return encoded + data + pad


def main() -> None:
    for item in REQUIRED:
        require(item)
    verify_addin_web_synced()

    if BUILD.exists():
        shutil.rmtree(BUILD)
    OUT.mkdir(parents=True, exist_ok=True)
    DEBIAN.mkdir(parents=True, exist_ok=True)
    DATA.mkdir(parents=True, exist_ok=True)

    normalize_control()
    for script in ["preinst", "postinst", "prerm", "postrm"]:
        write_debian_script(script)

    app_root = DATA / APP_ROOT_REL
    (app_root / "daemon").mkdir(parents=True, exist_ok=True)
    shutil.copy2(resolve_daemon_binary(), app_root / "daemon/wps-tts-daemon")
    write_version_json(app_root / "version.json")
    copytree_contents(ROOT / "addin", app_root / "addin")
    (app_root / "engines").mkdir(parents=True, exist_ok=True)
    copytree_contents(sherpa_runtime_source(), app_root / "engines/sherpa-onnx")
    (app_root / "voices").mkdir(parents=True, exist_ok=True)
    copytree_contents(ROOT / "voices" / "sherpa", app_root / "voices/sherpa")
    replace_duplicate_libraries_with_links()
    (DATA / CONFIG_REL).parent.mkdir(parents=True, exist_ok=True)
    write_linux_config(DATA / CONFIG_REL)
    (DATA / "lib/systemd/system").mkdir(parents=True, exist_ok=True)
    write_systemd_service(DATA / f"lib/systemd/system/{SERVICE_NAME}")
    (DATA / "usr/bin").mkdir(parents=True, exist_ok=True)
    register = DATA / f"usr/bin/{REGISTER_BIN_NAME}"
    write_debian_script("wps-read-aloud-register")
    shutil.move(DEBIAN / "wps-read-aloud-register", register)
    doc_dir = DATA / DOC_REL
    doc_dir.mkdir(parents=True, exist_ok=True)
    for doc in DOC_FILES:
        shutil.copy2(ROOT / "third_party_licenses" / doc, doc_dir / doc)
    for doc in PROJECT_DOC_FILES:
        shutil.copy2(ROOT / doc, doc_dir / Path(doc).name)

    debian_binary = b"2.0\n"
    control = tar_bytes(DEBIAN, ["control", "preinst", "postinst", "prerm", "postrm"], gzip_output=True)
    data = tar_bytes(DATA, all_data_names(), gzip_output=True, symlinks=DUPLICATE_LIBRARY_LINKS)

    with DEB.open("wb") as fh:
        fh.write(b"!<arch>\n")
        fh.write(ar_member("debian-binary", debian_binary))
        fh.write(ar_member("control.tar.gz", control))
        fh.write(ar_member("data.tar.gz", data))

    print(f"created {DEB}")


if __name__ == "__main__":
    main()
