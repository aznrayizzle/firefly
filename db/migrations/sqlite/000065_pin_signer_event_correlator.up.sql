ALTER TABLE pins ADD COLUMN signer TEXT;
UPDATE pins SET signer = '';

ALTER TABLE events ADD COLUMN cid UUID;
