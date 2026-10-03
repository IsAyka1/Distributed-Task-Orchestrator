package workflows

import _ "embed"

//go:embed insert_workflow.sql
var insertWorkflowSQL string

//go:embed insert_task.sql
var insertTaskSQL string

//go:embed enqueue.sql
var enqueueSQL string
