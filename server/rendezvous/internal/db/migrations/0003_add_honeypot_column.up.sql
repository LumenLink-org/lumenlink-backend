-- Add honeypot flag to gateways
ALTER TABLE gateways
ADD COLUMN is_honeypot BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_gateways_honeypot_region
ON gateways (is_honeypot, region)
WHERE is_honeypot = TRUE;

COMMENT ON COLUMN gateways.is_honeypot IS 'Indicates if this gateway is a honeypot for suspicious clients';
