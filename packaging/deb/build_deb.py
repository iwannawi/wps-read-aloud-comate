#!/usr/bin/env python3
import gzip
import io
import json
import os
import shutil
import tarfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PKG_NAME = "wps-read-aloud-xc"
ARTIFACT_NAME = "wps-read-aloud-xc"
VERSION = os.environ.get("VERSION", "1.0.24")
RELEASE_DATE = os.environ.get("RELEASE_DATE", "20260518")
ARCH = os.environ.get("ARCH", "arm64")
BUILD = ROOT / "build" / "deb" / f"{PKG_NAME}_{VERSION}_{ARCH}"
DATA = BUILD / "data"
DEBIAN = BUILD / "DEBIAN"
OUT = ROOT / "dist"
DEB = OUT / f"{ARTIFACT_NAME}_{VERSION}_{ARCH}.deb"
ADDIN = ROOT / "addin"
EMBEDDED_WEB = ROOT / "daemon" / "cmd" / "wps-tts-daemon" / "web"


REQUIRED = [
    "engines/sherpa-onnx/sherpa-onnx-offline-tts",
    "engines/sherpa-onnx/lib",
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
    "CHECKSUMS.txt",
    "docs/BUILD_RELEASE_LESSONS.md",
]

EXECUTABLE_SUFFIXES = {
    "opt/wps-read-aloud/daemon/wps-tts-daemon",
    "opt/wps-read-aloud/engines/sherpa-onnx/sherpa-onnx-offline-tts",
    "usr/bin/wps-read-aloud-register",
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
    "CHECKSUMS.txt",
    "docs/BUILD_RELEASE_LESSONS.md",
]

DUPLICATE_LIBRARY_LINKS = {}

EXCLUDED_PACKAGE_FILES = {
    "opt/wps-read-aloud/engines/.gitkeep",
    "opt/wps-read-aloud/engines/README.md",
    "opt/wps-read-aloud/voices/.gitkeep",
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
            member = tar.getmember("opt/wps-read-aloud/daemon/wps-tts-daemon")
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
    daemon = OUT / "wps-tts-daemon"
    if daemon.is_file():
        return daemon
    candidates = sorted(
        OUT.glob(f"{ARTIFACT_NAME}_*_{ARCH}.deb"),
        key=lambda path: path.stat().st_mtime,
        reverse=True,
    )
    for candidate in candidates:
        if extract_daemon_from_deb(candidate, daemon):
            return daemon
    raise SystemExit(
        "missing required file: dist/wps-tts-daemon; "
        "no previous package was available to reuse the daemon binary"
    )


def write_version_json(target: Path) -> None:
    info = {
        "name": "WPS 文档朗读助手",
        "package": PKG_NAME,
        "version": VERSION,
        "release_date": RELEASE_DATE,
    }
    target.write_text(json.dumps(info, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


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
        if line.startswith("Version:"):
            out.append(f"Version: {VERSION}")
        elif line.startswith("Architecture:"):
            out.append(f"Architecture: {ARCH}")
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
        shutil.copy2(ROOT / "packaging" / "deb" / script, DEBIAN / script)

    (DATA / "opt/wps-read-aloud/daemon").mkdir(parents=True, exist_ok=True)
    shutil.copy2(resolve_daemon_binary(), DATA / "opt/wps-read-aloud/daemon/wps-tts-daemon")
    write_version_json(DATA / "opt/wps-read-aloud/version.json")
    copytree_contents(ROOT / "addin", DATA / "opt/wps-read-aloud/addin")
    (DATA / "opt/wps-read-aloud/engines").mkdir(parents=True, exist_ok=True)
    copytree_contents(ROOT / "engines" / "sherpa-onnx", DATA / "opt/wps-read-aloud/engines/sherpa-onnx")
    (DATA / "opt/wps-read-aloud/voices").mkdir(parents=True, exist_ok=True)
    copytree_contents(ROOT / "voices" / "sherpa", DATA / "opt/wps-read-aloud/voices/sherpa")
    replace_duplicate_libraries_with_links()
    (DATA / "etc/wps-read-aloud").mkdir(parents=True, exist_ok=True)
    shutil.copy2(ROOT / "daemon" / "config.example.yaml", DATA / "etc/wps-read-aloud/config.yaml")
    (DATA / "lib/systemd/system").mkdir(parents=True, exist_ok=True)
    shutil.copy2(ROOT / "packaging" / "deb" / "wps-tts.service", DATA / "lib/systemd/system/wps-tts.service")
    (DATA / "usr/bin").mkdir(parents=True, exist_ok=True)
    shutil.copy2(ROOT / "packaging" / "deb" / "wps-read-aloud-register", DATA / "usr/bin/wps-read-aloud-register")
    doc_dir = DATA / "usr/share/doc/wps-read-aloud-xc"
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
