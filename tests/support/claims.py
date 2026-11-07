from dataclasses import dataclass, asdict
import json


@dataclass
class ClaimRequest:
    worker_id: str
    lease_duration: str
    limit: int | None = None


@dataclass
class Claims:
    application: object

    def request(self, worker="worker", duration="1m", limit=None, **environment):
        request = ClaimRequest(worker, duration, limit)
        fields = asdict(request)
        if limit is None:
            fields.pop("limit")
        return self.application.run("-claim", binary="probe",
                                    input=json.dumps(fields), **environment)

    def claim(self, **kwargs):
        result = self.request(**kwargs)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)
