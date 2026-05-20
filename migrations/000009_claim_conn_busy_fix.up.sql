-- 000009_claim_conn_busy_fix
-- Code-only migration marker for 0.6.3.2-alpha.
-- Fixes pgx "conn busy" in job claiming by closing candidate rows before issuing lock/update commands.
select 1;
