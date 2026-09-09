-- +migrate Up
-- Defensive cleanup for docker inventory rows whose parent server is gone.
-- Normal deletes use ON DELETE CASCADE; this heals older drift / partial deletes.

DELETE FROM container_metrics_raw c
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id);

DELETE FROM server_metrics_raw m
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id);

DELETE FROM server_metrics_5m m
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id);

DELETE FROM server_metrics_1h m
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id);

DELETE FROM agent_commands c
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id);

DELETE FROM containers c
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id);

DELETE FROM images i
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = i.server_id);

DELETE FROM volumes v
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = v.server_id);

DELETE FROM networks n
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = n.server_id);

DELETE FROM compose_projects p
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = p.server_id);

DELETE FROM docker_hosts d
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = d.server_id);

DELETE FROM agents a
 WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = a.server_id);
