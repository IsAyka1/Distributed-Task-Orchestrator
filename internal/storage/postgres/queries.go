package postgres

import _ "embed"

//go:embed database_now.sql
var databaseNowSQL string
