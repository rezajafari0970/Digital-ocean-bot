CREATE TABLE account_transport_state (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    transport_epoch BIGINT NOT NULL DEFAULT 1 CHECK (transport_epoch >= 1),
    active_proxy_id UUID REFERENCES proxies(id) ON DELETE SET NULL,
    transition_reason TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
SELECT a.id,a.provider,1,np.proxy_id,'migration-000108'
FROM accounts a
LEFT JOIN network_profiles np ON np.account_id=a.id
ON CONFLICT(account_id) DO NOTHING;
