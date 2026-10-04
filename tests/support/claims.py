from dataclasses import dataclass
import json


@dataclass
class Claims:
    application: object

    def request(self, worker="worker", duration="1m", **environment):
        return self.application.run("-claim", binary="probe",
                                    input=json.dumps(dict(worker_id=worker, lease_duration=duration)), **environment)

    def claim(self, **kwargs):
        result = self.request(**kwargs)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)
