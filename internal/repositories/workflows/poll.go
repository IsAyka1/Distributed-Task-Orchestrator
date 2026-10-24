package workflows

import (
	"context"
	"database/sql"
	"errors"
)

// Discovery takes no locks; evaluation rechecks the signal under the workflow lock.
func (r Repository) NextWakeup(ctx context.Context) (string, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, nextWakeupSQL).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
