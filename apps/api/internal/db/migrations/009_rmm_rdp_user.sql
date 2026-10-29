-- Store RDP username on host; password stays in envelope secrets (kind=rmm_rdp).
ALTER TABLE rmm_hosts
    ADD COLUMN IF NOT EXISTS rdp_username TEXT NOT NULL DEFAULT '';
