SELECT id, task_key, status, input, output, attempt_count, max_attempt_count, current_attempt_id
FROM orchestrator.task_runs WHERE workflow_run_id = $1;
