package adminapi

import "context"

func (s *Server) invalidateProxyCredentialEpoch(ctx context.Context, proxyID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE account_transport_state ats
SET transport_epoch=ats.transport_epoch+1,transition_reason='proxy-credential-change',updated_at=now()
FROM network_profiles np
WHERE np.account_id=ats.account_id AND np.proxy_id=$1::uuid`, proxyID)
	return err
}
