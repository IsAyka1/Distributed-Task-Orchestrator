package tasks

import _ "embed"

//go:embed lock_workflow.sql
var lockWorkflowSQL string

//go:embed lock_wakeup.sql
var lockWakeupSQL string

//go:embed lock_task.sql
var lockTaskSQL string

//go:embed lock_attempt.sql
var lockAttemptSQL string

//go:embed now.sql
var nowSQL string

//go:embed close_attempt.sql
var closeAttemptSQL string

//go:embed complete_task.sql
var completeTaskSQL string

//go:embed enqueue.sql
var enqueueSQL string

//go:embed increment_version.sql
var incrementVersionSQL string
