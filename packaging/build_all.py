#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "packaging" / "platforms.json"
PYTHON = sys.executable
DIST = ROOT / "dist"
INTERMEDIATE_PREFIXES = (
    "wps-tts-daemon-linux-",
    "wps-tts-daemon-windows-",
)
RELEASE_FILE_PREFIXES = (
    "wps-read-aloud-comate_",
    "cn.wps-read-aloud-comate_",
)


def load_targets():
    data = json.loads(MATRIX.read_text(encoding="utf-8"))
    return data["targets"]


def run(cmd, env=None):
    print("+", " ".join(str(item) for item in cmd))
    subprocess.run(cmd, cwd=ROOT, env=env, check=True)


def go_exe():
    bundled = ROOT / "tools" / "go" / "bin" / ("go.exe" if os.name == "nt" else "go")
    if bundled.exists():
        return bundled
    found = shutil.which("go")
    if found:
        return Path(found)
    return None


def build_linux_daemon(arch):
    out = DIST / f"wps-tts-daemon-linux-{arch}"
    if out.exists():
        return out
    go = go_exe()
    if go is None:
        raise SystemExit("missing Go toolchain; cannot build Linux daemon")
    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = arch
    env["CGO_ENABLED"] = "0"
    env.setdefault("GOCACHE", str(ROOT / "build" / "gocache"))
    DIST.mkdir(parents=True, exist_ok=True)
    print("+", " ".join([str(go), "build", "-buildvcs=false", "-o", str(out), "./cmd/wps-tts-daemon"]))
    subprocess.run(
        [str(go), "build", "-buildvcs=false", "-o", str(out), "./cmd/wps-tts-daemon"],
        cwd=ROOT / "daemon",
        env=env,
        check=True,
    )
    return out


def write_sha256(path):
    digest = hashlib.sha256(path.read_bytes()).hexdigest().upper()
    sha = path.with_name(path.name + ".sha256")
    sha.write_text(f"{digest}  dist\\{path.name}\n", encoding="ascii")
    return sha


def artifact_path(target):
    return DIST / target["artifact"]


def build_linux(target):
    build_linux_daemon(target["arch"])
    env = os.environ.copy()
    env["DISTRO"] = target["distro"]
    env["ARCH"] = target["arch"]
    run([PYTHON, "packaging/deb/build_deb.py"], env=env)
    write_sha256(artifact_path(target))


def build_windows(target):
    arch_label = "x86" if target["arch"] in {"386", "x86"} else target["arch"]
    daemon = DIST / f"wps-tts-daemon-windows-{arch_label}.exe"
    if not daemon.exists():
        go = go_exe()
        if go is None:
            raise SystemExit("missing Go toolchain; cannot build Windows daemon")
        env = os.environ.copy()
        env["GOOS"] = "windows"
        env["GOARCH"] = "386" if arch_label == "x86" else target["arch"]
        env["CGO_ENABLED"] = "0"
        env.setdefault("GOCACHE", str(ROOT / "build" / "gocache"))
        DIST.mkdir(parents=True, exist_ok=True)
        print("+", " ".join([str(go), "build", "-buildvcs=false", "-o", str(daemon), "./cmd/wps-tts-daemon-windows"]))
        subprocess.run(
            [str(go), "build", "-buildvcs=false", "-o", str(daemon), "./cmd/wps-tts-daemon-windows"],
            cwd=ROOT / "daemon",
            env=env,
            check=True,
        )
    env = os.environ.copy()
    env["WINDOWS_ARCH"] = target["arch"]
    run([PYTHON, "packaging/windows/build_windows_package.py"], env=env)
    write_sha256(artifact_path(target))


def validate_artifacts(targets):
    missing = []
    for target in targets:
        artifact = artifact_path(target)
        sha = artifact.with_name(artifact.name + ".sha256")
        if not artifact.is_file():
            missing.append(str(artifact.relative_to(ROOT)))
        if not sha.is_file():
            missing.append(str(sha.relative_to(ROOT)))
    if missing:
        raise SystemExit("release artifacts are incomplete:\n" + "\n".join("  " + item for item in missing))


def cleanup_intermediate_binaries():
    if not DIST.is_dir():
        return
    for path in DIST.iterdir():
        if path.is_file() and any(path.name.startswith(prefix) for prefix in INTERMEDIATE_PREFIXES):
            path.unlink()


def cleanup_stale_release_files():
    if not DIST.is_dir():
        return
    for path in DIST.iterdir():
        if path.is_file() and any(path.name.startswith(prefix) for prefix in RELEASE_FILE_PREFIXES):
            path.unlink()


def check_platform_inputs_if_all_targets(targets, selected_targets):
    if {target["id"] for target in targets} == {target["id"] for target in selected_targets}:
        run([PYTHON, "packaging/check_platform_inputs.py"])


def write_checksums(targets):
    lines = []
    for target in targets:
        artifact = artifact_path(target)
        sha = artifact.with_name(artifact.name + ".sha256")
        if sha.is_file():
            lines.append(sha.read_text(encoding="ascii").strip())
    (ROOT / "CHECKSUMS.txt").write_text("\n".join(lines) + "\n", encoding="ascii")


def verify_release_artifacts_if_all_targets(targets, selected_targets):
    if {target["id"] for target in targets} == {target["id"] for target in selected_targets}:
        run([PYTHON, "packaging/verify_release_artifacts.py"])


def main():
    parser = argparse.ArgumentParser(description="Build WPS read-aloud packages for selected targets.")
    parser.add_argument("--target", action="append", help="Target id. Can be used more than once.")
    parser.add_argument("--list", action="store_true", help="List supported targets.")
    parser.add_argument("--no-checksums", action="store_true", help="Do not refresh CHECKSUMS.txt.")
    args = parser.parse_args()

    targets = load_targets()
    if args.list:
        for target in targets:
            print(f"{target['id']}: {target['artifact']}")
        return

    selected = set(args.target or [target["id"] for target in targets])
    unknown = selected - {target["id"] for target in targets}
    if unknown:
        raise SystemExit("unknown target: " + ", ".join(sorted(unknown)))

    selected_targets = [target for target in targets if target["id"] in selected]
    if {target["id"] for target in targets} == {target["id"] for target in selected_targets}:
        cleanup_stale_release_files()
    check_platform_inputs_if_all_targets(targets, selected_targets)

    for target in selected_targets:
        if target["id"] not in selected:
            continue
        if target["os"] == "linux":
            build_linux(target)
        elif target["os"] == "windows":
            build_windows(target)
        else:
            raise SystemExit("unsupported target os: " + target["os"])
    validate_artifacts(selected_targets)
    if not args.no_checksums:
        write_checksums(selected_targets)
    cleanup_intermediate_binaries()
    verify_release_artifacts_if_all_targets(targets, selected_targets)


if __name__ == "__main__":
    main()
