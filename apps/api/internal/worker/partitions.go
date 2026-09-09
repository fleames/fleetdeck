package worker

import (
	"context"
	"fmt"
	"log"
	"time"
)

// monthBounds returns UTC [start, end) for the month containing t, and a safe partition suffix.
func monthBounds(t time.Time) (start, end time.Time, suffix string) {
	u := t.UTC()
	start = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	suffix = fmt.Sprintf("%04d_%02d", start.Year(), int(start.Month()))
	return start, end, suffix
}

func partitionTableName(parent, suffix string) string {
	return parent + "_" + suffix
}

// MonthsFullyBefore reports whether a calendar month ending at monthEnd is entirely at-or-before cutoff.
func MonthsFullyBefore(monthEnd, cutoff time.Time) bool {
	return !monthEnd.After(cutoff)
}

// ensureMetricsPartitions creates monthly RANGE partitions for the current and next month.
// New inserts land on named partitions when their ts falls in range; older DEFAULT data remains until DELETE.
func (r *Runner) ensureMetricsPartitions(ctx context.Context) {
	now := time.Now().UTC()
	months := []time.Time{now, now.AddDate(0, 1, 0)}
	parents := []string{"server_metrics_raw", "container_metrics_raw"}
	for _, m := range months {
		start, end, suffix := monthBounds(m)
		for _, parent := range parents {
			name := partitionTableName(parent, suffix)
			// Partition names are generated solely from year/month integers — never from user input.
			sql := fmt.Sprintf(
				`CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
				name, parent,
				start.Format("2006-01-02"),
				end.Format("2006-01-02"),
			)
			if _, err := r.pool.Exec(ctx, sql); err != nil {
				log.Printf("ensure partition %s: %v", name, err)
			}
		}
	}
}

// dropAgedRawPartitions drops named monthly partitions whose entire month is older than cutoff.
// Leaves DEFAULT partition alone (still cleaned via DELETE).
func (r *Runner) dropAgedRawPartitions(ctx context.Context, cutoff time.Time) {
	parents := []string{"server_metrics_raw", "container_metrics_raw"}
	cursor := time.Date(cutoff.UTC().Year(), cutoff.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 36; i++ {
		monthStart := cursor.AddDate(0, -i, 0)
		monthEnd := monthStart.AddDate(0, 1, 0)
		if !MonthsFullyBefore(monthEnd, cutoff) {
			continue
		}
		_, _, suffix := monthBounds(monthStart)
		for _, parent := range parents {
			name := partitionTableName(parent, suffix)
			var exists bool
			_ = r.pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_class c
					JOIN pg_namespace n ON n.oid = c.relnamespace
					WHERE n.nspname = 'public' AND c.relname = $1
				)`, name).Scan(&exists)
			if !exists {
				continue
			}
			sql := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)
			if _, err := r.pool.Exec(ctx, sql); err != nil {
				log.Printf("drop partition %s: %v", name, err)
			} else {
				log.Printf("dropped aged metrics partition %s", name)
			}
		}
	}
}
