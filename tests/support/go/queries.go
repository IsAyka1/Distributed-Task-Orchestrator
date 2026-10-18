package main

import _ "embed"

//go:embed worker_locks.sql
var workerLocksSQL string
