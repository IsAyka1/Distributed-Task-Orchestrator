from dataclasses import dataclass
import json


@dataclass
class Workflows:
    application: object

    def request(self, definition_id, **fields):
        return self.application.run("-workflow", "start", binary="probe",
                                    input=json.dumps(dict(definition_id=definition_id, **fields)))

    def start(self, definition_id, **fields):
        result = self.request(definition_id, **fields)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)

    def evaluate(self, workflow_id, **environment):
        return self.application.run("-workflow", "evaluate", binary="probe",
                                    input=json.dumps(dict(workflow_id=workflow_id)), **environment)
