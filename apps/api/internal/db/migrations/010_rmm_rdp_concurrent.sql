-- Host SKU / concurrent RDP capability (Windows Server vs client Pro).
-- Client Windows cannot legally host two interactive desktops; Server can.
ALTER TABLE rmm_hosts
    ADD COLUMN IF NOT EXISTS os_edition TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS os_product_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rdp_concurrent_sessions BOOLEAN NOT NULL DEFAULT false;
