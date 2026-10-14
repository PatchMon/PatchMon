WITH configured AS (
    SELECT regexp_replace(
               regexp_replace(btrim(oidc_issuer_url), '/\.well-known/openid-configuration/?$', ''),
               '/+$', '') AS issuer
    FROM settings
    WHERE oidc_enabled
      AND oidc_issuer_url IS NOT NULL
      AND (SELECT COUNT(*) FROM settings) = 1
),
parsed AS (
    SELECT issuer, lower(substring(issuer FROM '^[A-Za-z][A-Za-z0-9+.-]*://([^/:?#]+)')) AS host
    FROM configured
)
UPDATE users u
SET oidc_provider = p.issuer, updated_at = NOW()
FROM parsed p
WHERE p.host IS NOT NULL
  AND p.host <> ''
  AND u.oidc_sub IS NOT NULL
  AND u.oidc_provider IS NOT NULL
  AND position('://' IN u.oidc_provider) = 0
  AND lower(btrim(u.oidc_provider)) = p.host;
