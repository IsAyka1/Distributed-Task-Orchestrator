from dataclasses import dataclass, field
import subprocess

import launcher
import pytest


@dataclass
class ComposeFake:
    failure: str = ""
    interrupt: bool = False
    calls: list[tuple[str, ...]] = field(default_factory=list)
    runtime_values: list[str] = field(default_factory=list, repr=False)

    def run(self, args, **options):
        args = tuple(args)
        self.calls.append(args)
        output = ""
        if args[:2] == ("docker", "info"):
            output = "linux/amd64"
        elif args[:2] == ("docker", "compose"):
            self.runtime_values.append(options["env"]["TEST_DATABASE_PASSWORD"])
            if "up" in args and self.interrupt:
                raise KeyboardInterrupt
            if self.failure and self.failure in args:
                raise subprocess.CalledProcessError(1, args)
        return subprocess.CompletedProcess(args, 0, stdout=output, stderr="")

    def compose_calls(self):
        return [args for args in self.calls if args[:2] == ("docker", "compose")]


@pytest.fixture
def compose_fake(monkeypatch):
    fake = ComposeFake()
    monkeypatch.setattr(launcher.subprocess, "run", fake.run)
    monkeypatch.setattr(launcher.signal, "signal", lambda *args: None)
    return fake


def test_launcher_uses_compose(compose_fake):
    launcher.main()
    calls = compose_fake.compose_calls()
    assert any("--exit-code-from" in args and "tests" in args for args in calls)
    assert any("docker-compose.test.yml" in args[args.index("-f") + 1] for args in calls)
    assert not any(args[:2] in [("docker", "run"), ("docker", "create"), ("docker", "network"), ("docker", "rm")] for args in compose_fake.calls)


@pytest.mark.parametrize("operation", ["create", "cp", "up", "down"])
def test_compose_errors_fail_and_attempt_cleanup(compose_fake, operation):
    compose_fake.failure = operation
    with pytest.raises(subprocess.CalledProcessError):
        launcher.main()
    assert "down" in compose_fake.compose_calls()[-1]
    assert "--volumes" in compose_fake.compose_calls()[-1]


def test_compose_interruption_cleans_up(compose_fake):
    compose_fake.interrupt = True
    with pytest.raises(KeyboardInterrupt):
        launcher.main()
    assert "down" in compose_fake.compose_calls()[-1]


def test_compose_projects_are_isolated_and_runtime_values_stay_out_of_arguments(compose_fake):
    projects = []
    for _ in range(2):
        compose_fake.calls.clear()
        launcher.main()
        calls = compose_fake.compose_calls()
        names = {args[args.index("--project-name") + 1] for args in calls}
        assert len(names) == 1
        projects.extend(names)
        if any(value in part for value in compose_fake.runtime_values for args in compose_fake.calls for part in args):
            pytest.fail("runtime configuration was exposed in command arguments")
    assert projects[0] != projects[1]
    if not compose_fake.runtime_values:
        pytest.fail("runtime configuration was not generated")
