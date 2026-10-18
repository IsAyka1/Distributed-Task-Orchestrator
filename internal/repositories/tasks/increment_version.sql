UPDATE orchestrator.workflow_runs SET version = version + 1 WHERE id = $1;
