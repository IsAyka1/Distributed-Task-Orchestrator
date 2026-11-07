from dataclasses import asdict, dataclass
import json


@dataclass(frozen=True)
class WorkerRequest:
    Steps: int
    Now: str
    FailAt: str = ""


@dataclass
class Workers:
    application: object
    clock: object

    def request(self, count=5, fail_at="", **environment):
        request = WorkerRequest(count, self.clock.now.isoformat(), fail_at)
        return self.application.run("-worker", binary="probe", input=json.dumps(asdict(request)), **environment)

    def running(self, count=5, **environment):
        request = WorkerRequest(count, self.clock.now.isoformat())
        return self.application.running("-worker", binary="probe", input=json.dumps(asdict(request)), **environment)

    def step(self, count=5, fail_at=""):
        result = self.request(count, fail_at)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)
