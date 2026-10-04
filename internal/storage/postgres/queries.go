package postgres

import _ "embed"

//go:embed database_now.sql
var databaseNowSQL string

//go:embed claim_workflow.sql
var claimWorkflowSQL string

//go:embed claim_wakeup.sql
var claimWakeupSQL string

//go:embed claim_task.sql
var claimTaskSQL string

//go:embed claim_insert_attempt.sql
var claimInsertAttemptSQL string

//go:embed claim_update_task.sql
var claimUpdateTaskSQL string

//go:embed claim_enqueue.sql
var claimEnqueueSQL string

//go:embed claim_revision.sql
var claimRevisionSQL string
