from dataclasses import asdict, dataclass, field
from pathlib import Path
import json

import yaml


@dataclass(frozen=True)
class TaskData:
    id: str
    type: str
    depends_on: list[str] = field(default_factory=list)
    max_attempts: int = 1


@dataclass(frozen=True)
class DefinitionData:
    provider: str
    name: str
    version: int
    tasks: list[TaskData]

    @classmethod
    def from_yaml(cls, path: Path):
        with path.open(encoding="utf-8") as source:
            document = yaml.safe_load(source)
        tasks = [TaskData(**task) for task in document.pop("tasks")]
        return cls(**document, tasks=tasks)

    def document(self):
        return asdict(self)


@dataclass
class Definitions:
    application: object

    def request(self, action, document):
        return self.application.run("-definition", action, binary="probe", input=json.dumps(document))

    def succeed(self, action, document):
        result = self.request(action, document)
        assert result.returncode == 0, result.stderr
        return json.loads(result.stdout)
