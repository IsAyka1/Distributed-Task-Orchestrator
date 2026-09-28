import os
import secrets
from pathlib import Path
import signal
import subprocess
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[1]
POSTGRES = "postgres:18.3-bookworm@sha256:80630f83606d8db77d30b3851b16a9f78be2d0d4dda6f7b82a1fdca5ebe3acba"
PYTHON = "python:3.13.12-slim-bookworm@sha256:a58daefb915e1e03ad48f3ca4df8832065412c5c35cacb9d39f4229184de12b6"


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
    platform = "linux/" + arch
    run_id = "taskmanager-test-" + uuid.uuid4().hex
    database, runner = run_id + "-db", run_id + "-python"
    credential = secrets.token_urlsafe(32)
    database_env = dict(os.environ, POSTGRES_PASSWORD=credential)
    runner_env = dict(os.environ, TEST_DATABASE_PASSWORD=credential)
    signal.signal(signal.SIGTERM, interrupted)
    with tempfile.TemporaryDirectory(prefix="taskmanager-binaries-") as build:
        env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH=arch)
        for name, package in [("orchestrator", "./cmd/orchestrator"), ("probe", "./tests/support/go")]:
            command("go", "build", "-o", str(Path(build) / name), package, cwd=ROOT, env=env)
        try:
            command("docker", "network", "create", "--label", "org.taskmanager.test-run=" + run_id, run_id, stdout=subprocess.DEVNULL)
            command("docker", "run", "--platform", platform, "-d", "--name", database, "--label", "org.taskmanager.test-run=" + run_id,
                    "--network", run_id, "--network-alias", "database",
                    "--tmpfs", "/var/lib/postgresql", "-e", "POSTGRES_PASSWORD",
                    POSTGRES, env=database_env, stdout=subprocess.DEVNULL)
            command("docker", "create", "--platform", platform, "--name", runner, "--label", "org.taskmanager.test-run=" + run_id,
                    "--network", run_id, "-w", "/workspace", "-e", "PYTHONDONTWRITEBYTECODE=1",
                    "-e", "TEST_DATABASE_PASSWORD",
                    PYTHON, "sh", "-ec",
                    "pip install --disable-pip-version-check --no-cache-dir -r tests/requirements.txt && "
                    "python -m pytest -p no:cacheprovider -v tests/integration tests/failure",
                    env=runner_env, stdout=subprocess.DEVNULL)
            command("docker", "cp", str(ROOT / "tests"), runner + ":/workspace/tests")
            command("docker", "cp", str(Path(__file__).resolve()), runner + ":/workspace/launcher.py")
            command("docker", "cp", build, runner + ":/binaries")
            command("docker", "start", "-a", runner, timeout=600)
            result = command("docker", "inspect", "--format", "{{.State.ExitCode}}", runner,
                             capture_output=True, text=True)
            if result.stdout.strip() != "0":
                raise RuntimeError("Python test container failed with exit code " + result.stdout.strip())
        finally:
            # Only remove resources carrying this invocation's unguessable ownership label.
            failures = []
            for kind, name in [("container", runner), ("container", database), ("network", run_id)]:
                try:
                    field = ".Config.Labels" if kind == "container" else ".Labels"
                    template = '{{index ' + field + ' "org.taskmanager.test-run"}}'
                    result = subprocess.run(["docker", kind, "inspect", "--format", template, name],
                                            capture_output=True, text=True, timeout=15)
                    if result.returncode:
                        # A missing resource is expected after a failed create; daemon errors are not.
                        if "No such" not in result.stderr:
                            failures.append(result.stderr)
                        continue
                    if result.stdout.strip() != run_id:
                        failures.append(f"refusing to remove unowned {name}")
                        continue
                    args = ["docker", kind, "rm"] + (["-f", "-v"] if kind == "container" else []) + [name]
                    result = subprocess.run(args, capture_output=True, text=True, timeout=30)
                    if result.returncode:
                        failures.append(result.stderr)
                except (OSError, subprocess.SubprocessError) as error:
                    failures.append(f"{name}: {error}")
            if failures:
                raise RuntimeError("test cleanup failed: " + "; ".join(failures))


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"Python harness failed: {error}. Requires Go, Python 3.10+, Docker and registry access.")
