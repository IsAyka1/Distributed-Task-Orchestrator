package definitions

import _ "embed"

//go:embed insert.sql
var insertSQL string

//go:embed find.sql
var findSQL string

//go:embed find_by_id.sql
var findByIDSQL string
