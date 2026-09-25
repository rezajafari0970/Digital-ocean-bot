-- Scheduler wake-up is an internal DB-only tick. Keep it at the schema-safe
-- minimum; actual CREATE cadence is controlled by accounts.next_build_at.
UPDATE accounts SET auto_interval_seconds=60 WHERE auto_interval_seconds<60;
UPDATE schedules SET interval_seconds=60 WHERE interval_seconds<60;
