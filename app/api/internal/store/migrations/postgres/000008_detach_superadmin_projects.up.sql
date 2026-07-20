-- Detach superadmins from every project.
--
-- A superadmin is the platform-wide god-role: they can view and manage every
-- project purely via users.role, with no project_members row required (see the
-- superadmin bypass in the API authz middleware). Historically each account —
-- superadmins included — was auto-provisioned a "personal" project and added as
-- its admin, which left superadmins attached to a single project they are not
-- conceptually tied to.
--
-- Going forward superadmins are never provisioned a personal project; this
-- backfills existing data by removing every membership row belonging to a
-- superadmin. Their orphaned personal projects are intentionally left in place
-- (a superadmin still sees and can manage them via the global role); only the
-- membership link is dropped.
DELETE FROM project_members
 WHERE user_id IN (SELECT id FROM users WHERE role = 'superadmin');
