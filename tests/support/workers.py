from dataclasses import dataclass
import json


@dataclass
class Workers:
    application: object
    clock: object

    def step(self, count=5, fail_at=""):
        result = self.application.run("-worker", binary="probe", input=json.dumps({
            "Steps": count, "Now": self.clock.now.isoformat(), "FailAt": fail_at}))
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)
