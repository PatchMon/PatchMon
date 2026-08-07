DELETE FROM alert_config WHERE alert_type IN (
    'host_security_updates_exceeded', 'host_pending_updates_exceeded',
    'host_security_updates_resolved', 'host_pending_updates_resolved'
);
