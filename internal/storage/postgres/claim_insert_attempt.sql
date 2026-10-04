INSERT INTO orchestrator.task_attempts
    (id, task_run_id, attempt_no, worker_id, status, started_at, heartbeat_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $5)
RETURNING id, task_run_id, worker_id, attempt_no, status, started_at, heartbeat_at;
