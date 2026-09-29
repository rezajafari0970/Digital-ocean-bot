UPDATE accounts
SET deleted_at=COALESCE(deleted_at,now()),
    runtime_status_detail=CASE WHEN runtime_status='DELETE_PENDING' THEN 'archived; provider cleanup pending' ELSE runtime_status_detail END,
    updated_at=now()
WHERE deletion_requested_at IS NOT NULL AND deleted_at IS NULL;
