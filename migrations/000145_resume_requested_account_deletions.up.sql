-- Upgrade already requested deletions; no new account is selected.
INSERT INTO account_deletion_jobs(account_id,requested_at)
 SELECT id,deletion_requested_at FROM accounts
 WHERE NOT enabled AND deletion_requested_at IS NOT NULL AND runtime_status='DELETE_PENDING'
 ON CONFLICT DO NOTHING;
