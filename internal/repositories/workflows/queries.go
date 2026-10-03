package workflows

import _ "embed"

//go:embed insert_workflow.sql
var insertWorkflowSQL string

//go:embed insert_task.sql
var insertTaskSQL string

//go:embed enqueue.sql
var enqueueSQL string

//go:embed lock_workflow.sql
var lockWorkflowSQL string

//go:embed lock_wakeup.sql
var lockWakeupSQL string

//go:embed lock_tasks.sql
var lockTasksSQL string

//go:embed lock_attempts.sql
var lockAttemptsSQL string

//go:embed evaluation_time.sql
var evaluationTimeSQL string

//go:embed activate_task.sql
var activateTaskSQL string

//go:embed update_evaluation.sql
var updateEvaluationSQL string

//go:embed delete_wakeup.sql
var deleteWakeupSQL string
