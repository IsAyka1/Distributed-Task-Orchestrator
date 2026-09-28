import os
from pathlib import Path
import secrets
import signal
import subprocess
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]


def command(*args, timeout=300, **kwargs):
    return subprocess.run(args, check=True, timeout=timeout, **kwargs)


def interrupted(signum, frame):
    raise KeyboardInterrupt


def main():
    info = command("docker", "info", "--format", "{{.OSType}}/{{.Architecture}}",
                   timeout=15, capture_output=True, text=True).stdout.strip()
    system, _, architecture = info.partition("/")
    arch = {"amd64": "amd64", "x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(architecture)
    if system != "linux" or arch is None:
        raise RuntimeError("Python harness requires an amd64 or arm64 Linux Docker daemon")
    project = "taskmanager-test-" + uuid.uuid4().hex
    runtime = dict(os.environ, TEST_PLATFORM="linux/" + arch,
                   TEST_DATABASE_PASSWORD=secrets.token_urlsafe(32))
    prefix = ("docker", "compose", "--env-file", os.devnull, "-f",
              str(ROOT / "docker-compose.test.yml"), "--project-name", project)

    def compose(*args, **options):
        return command(*prefix, *args, env=runtime, **options)

    compose("config", "--quiet", timeout=15)
    signal.signal(signal.SIGTERM, interrupted)
    with tempfile.TemporaryDirectory(prefix="taskmanager-binaries-") as build:
        env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH=arch)
        for name, package in [("orchestrator", "./cmd/orchestrator"), ("probe", "./tests/support/go")]:
            command("go", "build", "-o", str(Path(build) / name), package, cwd=ROOT, env=env)
        try:
            compose("create")
            compose("cp", str(ROOT / "tests"), "tests:/workspace/tests")
            compose("cp", str(Path(__file__).resolve()), "tests:/workspace/launcher.py")
            compose("cp", build, "tests:/binaries")
            compose("up", "--no-recreate", "--abort-on-container-exit", "--exit-code-from", "tests",
                    "--attach", "tests", "--no-color", "--timeout", "5", timeout=600)
        finally:
            compose("down", "--volumes", "--timeout", "5", timeout=60)


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"Python harness failed: {error}. Requires Go, Python 3.10+, Docker Compose v2 and registry access.")
