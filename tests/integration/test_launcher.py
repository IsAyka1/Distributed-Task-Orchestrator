from dataclasses import dataclass, field
import subprocess

import launcher
import pytest


@dataclass
class DockerFake:
    fail_cleanup: str = ""
    run_id: str = ""
    calls: list[tuple[str, ...]] = field(default_factory=list)
    build_arches: list[str] = field(default_factory=list)
    failed: bool = False
    runtime_credential: str = field(default="", repr=False)

    def run(self, args, **options):
        args = tuple(args)
        self.calls.append(args)
        output = ""
        if args[:2] == ("docker", "info"):
            output = "linux/aarch64"
        elif args[:2] == ("docker", "run"):
            self.runtime_credential = options["env"]["POSTGRES_PASSWORD"]
        elif args[:2] == ("docker", "create"):
            if options["env"]["TEST_DATABASE_PASSWORD"] != self.runtime_credential:
                pytest.fail("test containers received inconsistent runtime configuration")
        elif args[:2] == ("go", "build"):
            self.build_arches.append(options["env"]["GOARCH"])
        elif args[:3] == ("docker", "network", "create"):
            self.run_id = args[-1]
        elif args[:2] == ("docker", "start") and self.fail_cleanup:
            raise subprocess.CalledProcessError(1, args)
        elif args[:2] == ("docker", "inspect"):
            output = "0"
        elif len(args) > 2 and args[0] == "docker" and args[2] in ("inspect", "rm"):
            if args[2] == self.fail_cleanup and not self.failed:
                self.failed = True
                raise subprocess.TimeoutExpired(args, 15)
            if args[2] == "inspect":
                output = self.run_id
        return subprocess.CompletedProcess(args, 0, stdout=output, stderr="")


@pytest.fixture
def docker_fake(monkeypatch):
    fake = DockerFake()
    monkeypatch.setattr(launcher.subprocess, "run", fake.run)
    monkeypatch.setattr(launcher.signal, "signal", lambda *args: None)
    return fake


def test_launcher_targets_daemon_architecture(docker_fake):
    launcher.main()
    assert docker_fake.build_arches == ["arm64", "arm64"]
    containers = [args for args in docker_fake.calls if args[:2] in [("docker", "run"), ("docker", "create")]]
    assert len(containers) == 2
    for args in containers:
        assert args[args.index("--platform") + 1] == "linux/arm64"


@pytest.mark.parametrize("operation", ["inspect", "rm"])
def test_cleanup_attempts_remaining_resources_after_timeout(docker_fake, operation):
    docker_fake.fail_cleanup = operation
    with pytest.raises(RuntimeError, match="test cleanup failed"):
        launcher.main()
    removals = [args for args in docker_fake.calls if args[:3] in [("docker", "container", "rm"), ("docker", "network", "rm")]]
    assert any(args[-1] == docker_fake.run_id + "-db" for args in removals)
    assert any(args[-1] == docker_fake.run_id for args in removals)


def test_launcher_keeps_runtime_configuration_out_of_arguments(docker_fake):
    launcher.main()
    if not docker_fake.runtime_credential:
        pytest.fail("test runtime configuration was not generated")
    if any(docker_fake.runtime_credential in part for args in docker_fake.calls for part in args):
        pytest.fail("test runtime configuration was exposed in command arguments")
