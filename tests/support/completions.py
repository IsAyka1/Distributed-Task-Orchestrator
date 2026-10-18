from dataclasses import dataclass
import json


@dataclass
class Completions:
    application: object

    def request(self, claim, event="SUCCEED", output=None, error_type="", error_message="", **environment):
        return self.application.run("-complete", binary="probe", input=json.dumps(dict(
            TaskID=claim["TaskID"], AttemptID=claim["Attempt"]["ID"], WorkerID=claim["Attempt"]["WorkerID"],
            Event=event, Output=dict(Value=output), ErrorType=error_type, ErrorMessage=error_message)), **environment)

    def complete(self, claim, **kwargs):
        result = self.request(claim, **kwargs)
        assert result.returncode == 0, result.stderr
        assert result.stdout == ""
