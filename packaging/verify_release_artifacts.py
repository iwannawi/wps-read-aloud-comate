#!/usr/bin/env python3
import hashlib
import io
import json
import tarfile
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "dist"
MATRIX = ROOT / "packaging" / "platforms.json"
CHECKSUMS = ROOT / "CHECKSUMS.txt"


FORBIDDEN_RESOURCE_TOKENS = (
    "piper",
    "espeak",
)

FORBIDDEN_PACKAGE_FILENAMES = (
    "ACCEPTANCE_TEST.md",
    "BUILD_RELEASE_LESSONS.md",
    "CODEX_AUTOMATION.md",
)


def fail(message: str) -> None:
    raise SystemExit(message)


def load_targets() -> list[dict]:
    return json.loads(MATRIX.read_text(encoding="utf-8"))["targets"]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest().upper()


def check_sha(path: Path) -> str:
    digest = sha256(path)
    sha_file = path.with_name(path.name + ".sha256")
    if not sha_file.is_file():
        fail(f"missing checksum file: {sha_file.relative_to(ROOT)}")
    expected = sha_file.read_text(encoding="ascii").strip()
    if expected != f"{digest}  dist\\{path.name}":
        fail(f"checksum mismatch: {sha_file.relative_to(ROOT)}")
    return expected


def ar_members(deb: Path) -> dict[str, bytes]:
    data = deb.read_bytes()
    if not data.startswith(b"!<arch>\n"):
        fail(f"not a valid deb ar file: {deb.relative_to(ROOT)}")
    pos = 8
    members = {}
    while pos + 60 <= len(data):
        header = data[pos : pos + 60]
        name = header[:16].decode("ascii").strip().rstrip("/")
        size = int(header[48:58].decode("ascii").strip())
        pos += 60
        members[name] = data[pos : pos + size]
        pos += size + (size % 2)
    for required in ("debian-binary", "control.tar.gz", "data.tar.gz"):
        if required not in members:
            fail(f"missing deb member {required}: {deb.relative_to(ROOT)}")
    return members


def tar_names(payload: bytes) -> tuple[tarfile.TarFile, set[str]]:
    tar = tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz")
    return tar, set(tar.getnames())


def check_no_forbidden(names: set[str], artifact: Path) -> None:
    lowered = [name.lower() for name in names]
    for token in FORBIDDEN_RESOURCE_TOKENS:
        matches = [name for name in lowered if token in name]
        if matches:
            fail(f"forbidden unused resource in {artifact.name}: {matches[0]}")
    for filename in FORBIDDEN_PACKAGE_FILENAMES:
        matches = [name for name in names if name.endswith("/" + filename) or name == filename]
        if matches:
            fail(f"internal project document leaked into {artifact.name}: {matches[0]}")


def check_linux_package(target: dict, artifact: Path) -> None:
    members = ar_members(artifact)
    control_tar, control_names = tar_names(members["control.tar.gz"])
    data_tar, data_names = tar_names(members["data.tar.gz"])
    if "control" not in control_names:
        fail(f"missing control file: {artifact.name}")
    control = control_tar.extractfile("control").read().decode("utf-8")
    if f"Architecture: {target['arch']}" not in control:
        fail(f"wrong deb architecture in control: {artifact.name}")
    if target["distro"] == "kylin" and "银河麒麟系统" not in control:
        fail(f"missing Kylin description in control: {artifact.name}")
    if target["distro"] == "uos" and "UOS系统" not in control:
        fail(f"missing UOS系统 description in control: {artifact.name}")

    app_root = "opt/wps-read-aloud-comate"
    config_path = "etc/wps-read-aloud-comate/config.yaml"
    package_name = "wps-read-aloud-comate"
    if target["distro"] == "uos":
        app_root = "opt/apps/cn.wps-read-aloud-comate/files"
        config_path = f"{app_root}/config.yaml"
        package_name = "cn.wps-read-aloud-comate"
        if not artifact.name.startswith("cn."):
            fail(f"UOS系统 package name must start with cn.: {artifact.name}")
    elif artifact.name.startswith("cn."):
        fail(f"Kylin package name must not use cn. prefix: {artifact.name}")
    required = {
        f"{app_root}/addin/assets/app.js",
        f"{app_root}/daemon/wps-tts-daemon",
        f"{app_root}/engines/sherpa-onnx/sherpa-onnx-offline-tts",
        f"{app_root}/voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx",
        f"{app_root}/version.json",
        config_path,
        "lib/systemd/system/wps-read-aloud-comate.service",
        "usr/bin/wps-read-aloud-comate-register",
    }
    missing = sorted(required - data_names)
    if missing:
        fail(f"missing Linux payload file in {artifact.name}: {missing[0]}")
    if f"Package: {package_name}" not in control:
        fail(f"wrong deb package name in control: {artifact.name}")
    version = json.loads(data_tar.extractfile(f"{app_root}/version.json").read().decode("utf-8"))
    if version.get("distro") != target["distro"] or version.get("architecture") != target["arch"]:
        fail(f"wrong version.json platform in {artifact.name}")
    if version.get("package") != package_name:
        fail(f"wrong version.json package name in {artifact.name}")
    if any(name.endswith(".exe") for name in data_names):
        fail(f"Windows executable leaked into Linux package: {artifact.name}")
    check_no_forbidden(data_names, artifact)


def check_windows_package(target: dict, artifact: Path) -> None:
    data = artifact.read_bytes()
    if not data.startswith(b"MZ"):
        fail(f"Windows artifact is not an exe installer: {artifact.name}")
    payload = find_embedded_zip(data, artifact)
    with zipfile.ZipFile(io.BytesIO(payload)) as zf:
        names = set(zf.namelist())
        required = {
            "install.ps1",
            "install-ui.ps1",
            "uninstall.ps1",
            "app/addin/assets/app.js",
            "app/addin/assets/runtime-config.js",
            "app/config.yaml",
            "app/daemon/wps-tts-daemon.exe",
            "app/engines/sherpa-onnx/sherpa-onnx-offline-tts.exe",
            "app/voices/sherpa/vits-zh-hf-fanchen-C/vits-zh-hf-fanchen-C.onnx",
            "app/version.json",
        }
        missing = sorted(required - names)
        if missing:
            fail(f"missing Windows payload file in {artifact.name}: {missing[0]}")
        version = json.loads(zf.read("app/version.json").decode("utf-8"))
        install_script = zf.read("install.ps1").decode("utf-8-sig")
        uninstall_script = zf.read("uninstall.ps1").decode("utf-8-sig")
        if "Register-DaemonStartup" not in install_script:
            fail(f"Windows installer does not register daemon startup: {artifact.name}")
        if "Wait-LocalServiceHealthy" not in install_script:
            fail(f"Windows installer does not verify the daemon started during install: {artifact.name}")
        if "jspluginonline name=" not in install_script or "http://127.0.0.1:19860/addin/" not in install_script:
            fail(f"Windows installer is not using publish online mode: {artifact.name}")
        if "New-OfflineAddinPackage" in install_script or "Set-WpsOemOfflineConfig" in install_script:
            fail(f"Windows installer still contains offline OEM package registration: {artifact.name}")
        if "WPSReadAloudComate" not in uninstall_script or "Uninstall" not in install_script:
            fail(f"Windows uninstall integration is incomplete: {artifact.name}")
        if (
            "WPS文档朗读助手" not in install_script
            or "卸载 WPS文档朗读助手.lnk" not in install_script
            or "打开安装目录.lnk" not in install_script
        ):
            fail(f"Windows Start Menu uninstall shortcut is not in the required folder/name: {artifact.name}")
        if "Remove-WpsOemConfig" not in uninstall_script:
            fail(f"Windows uninstaller does not clean OEM offline mode config: {artifact.name}")
    if version.get("system") != "windows" or version.get("architecture") != "x86":
        fail(f"wrong version.json platform in {artifact.name}")
    if any(name.endswith(".service") or name.startswith("lib/systemd/") for name in names):
        fail(f"Linux service file leaked into Windows package: {artifact.name}")
    if any(name.endswith(".so") for name in names):
        fail(f"Linux shared library leaked into Windows package: {artifact.name}")
    check_no_forbidden(names, artifact)


def find_embedded_zip(data: bytes, artifact: Path) -> bytes:
    marker = b"WPS_READ_ALOUD_COMATE_PAYLOAD_ZIP_V1\n"
    offset = data.rfind(marker)
    if offset < 0:
        fail(f"cannot locate embedded payload marker in {artifact.name}")
    payload = data[offset + len(marker) :]
    try:
        with zipfile.ZipFile(io.BytesIO(payload)) as zf:
            names = set(zf.namelist())
            if "install.ps1" in names and "app/version.json" in names:
                return payload
    except zipfile.BadZipFile:
        pass
    fail(f"embedded payload zip is invalid in {artifact.name}")


def main() -> None:
    targets = load_targets()
    checksum_lines = []
    expected_dist_files = set()
    for target in targets:
        artifact = DIST / target["artifact"]
        expected_dist_files.add(target["artifact"])
        expected_dist_files.add(target["artifact"] + ".sha256")
        if not artifact.is_file():
            fail(f"missing release artifact: {artifact.relative_to(ROOT)}")
        checksum_lines.append(check_sha(artifact))
        if target["os"] == "linux":
            check_linux_package(target, artifact)
        elif target["os"] == "windows":
            check_windows_package(target, artifact)
        else:
            fail(f"unsupported target os: {target['os']}")
    if not CHECKSUMS.is_file():
        fail("missing CHECKSUMS.txt")
    actual = CHECKSUMS.read_text(encoding="ascii").strip().splitlines()
    if actual != checksum_lines:
        fail("CHECKSUMS.txt does not match the five release artifacts")
    extra = sorted(path.name for path in DIST.iterdir() if path.is_file() and path.name not in expected_dist_files)
    if extra:
        fail("unexpected release file in dist: " + extra[0])
    print("release_artifacts_ok")


if __name__ == "__main__":
    main()
