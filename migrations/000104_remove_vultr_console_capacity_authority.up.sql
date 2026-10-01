UPDATE provider_capacity_observations
SET lower_bound=GREATEST(lower_bound,compute_limit),
    compute_limit=0,
    source='vultr_api_lower_bound',
    probe_after=NULL,
    probe_in_flight=false,
    updated_at=now()
WHERE source='vultr_console';
