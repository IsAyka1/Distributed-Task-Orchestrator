from dataclasses import dataclass
import json
from uuid import uuid4

from psycopg.types.json import Jsonb


@dataclass(frozen=True)
class Definition:
    provider: str = "demo"
    name: str = "sequence"
    version: int = 1
    content: str = '{"tasks":[{"id":"A","type":"activity","depends_on":[]}]}'

    def insert(self, conn):
        identifier = uuid4()
        conn.execute(
            "INSERT INTO orchestrator.workflow_definitions "
            "(id, provider, name, version, definition) VALUES (%s, %s, %s, %s, %s)",
            (identifier, self.provider, self.name, self.version, Jsonb(json.loads(self.content))),
        )
        return identifier


def insert_run(conn, definition_id, *, version=1, payload=None):
    identifier = uuid4()
    conn.execute(
        "INSERT INTO orchestrator.workflow_runs (id, definition_id, version, input) "
        "VALUES (%s, %s, %s, %s)",
        (identifier, definition_id, version, Jsonb(payload)),
    )
    return identifier
