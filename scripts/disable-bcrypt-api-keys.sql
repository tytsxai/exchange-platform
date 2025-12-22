-- Migration helper: disable legacy bcrypt API secrets that cannot be verified for HMAC.
-- Any API key with bcrypt-formatted secret_hash will be marked disabled (status=2).

UPDATE exchange_user.api_keys
SET status = 2,
    updated_at_ms = (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
WHERE secret_hash ~ '^\$2[aby]\$';
