-- Package A only. Run as the dedicated migration identity, never a runtime login.
-- Runtime logins are provisioned separately and inherit one of these NOLOGIN roles.
DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'foundation_api') THEN
    CREATE ROLE foundation_api NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'foundation_worker') THEN
    CREATE ROLE foundation_worker NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
  END IF;
END $$;

CREATE SCHEMA identity;
CREATE SCHEMA organizations;
CREATE SCHEMA audit;
CREATE SCHEMA eventing;
REVOKE ALL ON SCHEMA identity, organizations, audit, eventing FROM PUBLIC;

CREATE TABLE identity.users (
  id uuid PRIMARY KEY,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE organizations.organizations (
  id uuid PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE organizations.organization_memberships (
  organization_id uuid NOT NULL REFERENCES organizations.organizations(id),
  user_id uuid NOT NULL REFERENCES identity.users(id),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (organization_id, user_id)
);
CREATE TABLE organizations.workspaces (
  id uuid PRIMARY KEY,
  owner_kind text NOT NULL CHECK (owner_kind IN ('PERSON', 'ORGANIZATION')),
  owner_user_id uuid REFERENCES identity.users(id),
  owner_organization_id uuid REFERENCES organizations.organizations(id),
  name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name) AND name !~ '[[:cntrl:]]'),
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT exactly_one_typed_owner CHECK (
    (owner_kind = 'PERSON' AND owner_user_id IS NOT NULL AND owner_organization_id IS NULL) OR
    (owner_kind = 'ORGANIZATION' AND owner_user_id IS NULL AND owner_organization_id IS NOT NULL)
  )
);
CREATE TABLE organizations.workspace_memberships (
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  user_id uuid NOT NULL REFERENCES identity.users(id),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (workspace_id, user_id)
);
CREATE TABLE organizations.workspace_permissions (
  workspace_id uuid NOT NULL,
  user_id uuid NOT NULL,
  permission text NOT NULL CHECK (permission IN ('workspace.read', 'workspace.update')),
  PRIMARY KEY (workspace_id, user_id, permission),
  FOREIGN KEY (workspace_id, user_id) REFERENCES organizations.workspace_memberships(workspace_id, user_id)
);

-- Only this narrow helper can inspect the identity/membership tables from the API role.
-- Locking makes revocation and protected operations serialize at the membership boundary.
-- No dynamic SQL; search_path and all relation names are fixed.
CREATE FUNCTION organizations.authorize_workspace(required_permission text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE allowed boolean;
BEGIN
  SELECT true INTO allowed
  FROM identity.users u
  JOIN organizations.workspace_memberships m ON m.user_id = u.id
  JOIN organizations.workspace_permissions p ON p.workspace_id = m.workspace_id AND p.user_id = m.user_id
  WHERE u.id = nullif(current_setting('app.actor_id', true), '')::uuid
    AND m.workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid
    AND u.active AND m.active AND p.permission = required_permission
  FOR SHARE OF u, m, p;
  RETURN coalesce(allowed, false);
END $$;
REVOKE ALL ON FUNCTION organizations.authorize_workspace(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION organizations.authorize_workspace(text) TO foundation_api;

ALTER TABLE organizations.workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations.workspaces FORCE ROW LEVEL SECURITY;
CREATE POLICY workspace_read ON organizations.workspaces FOR SELECT TO foundation_api
USING (id = nullif(current_setting('app.workspace_id', true), '')::uuid AND organizations.authorize_workspace('workspace.read'));
CREATE POLICY workspace_update ON organizations.workspaces FOR UPDATE TO foundation_api
USING (id = nullif(current_setting('app.workspace_id', true), '')::uuid AND organizations.authorize_workspace('workspace.update'))
WITH CHECK (id = nullif(current_setting('app.workspace_id', true), '')::uuid AND organizations.authorize_workspace('workspace.update'));

CREATE TABLE audit.events (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  actor_user_id uuid NOT NULL REFERENCES identity.users(id),
  action text NOT NULL CHECK (action = 'workspace.name_updated'),
  target_kind text NOT NULL CHECK (target_kind = 'Workspace'),
  target_id uuid NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  request_id uuid NOT NULL,
  correlation_id uuid NOT NULL,
  result text NOT NULL CHECK (result = 'SUCCESS'),
  previous_version bigint NOT NULL CHECK (previous_version > 0),
  resource_version bigint NOT NULL,
  metadata jsonb NOT NULL,
  CHECK (target_id = workspace_id),
  CHECK (resource_version = previous_version + 1),
  CHECK (metadata = jsonb_build_object('previous_version', previous_version, 'version', resource_version)),
  UNIQUE (workspace_id, id, resource_version),
  UNIQUE (workspace_id, resource_version)
);
ALTER TABLE audit.events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit.events FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_append ON audit.events FOR INSERT TO foundation_api
WITH CHECK (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid
  AND actor_user_id = nullif(current_setting('app.actor_id', true), '')::uuid
  AND organizations.authorize_workspace('workspace.update'));

CREATE TABLE eventing.outbox (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  audit_event_id uuid NOT NULL,
  aggregate_version bigint NOT NULL CHECK (aggregate_version > 1),
  event_type text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 80),
  schema_version integer NOT NULL CHECK (schema_version > 0),
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 4096),
  occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  correlation_id uuid NOT NULL,
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'DELIVERED', 'DEAD')),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  lease_token uuid,
  leased_until timestamptz,
  delivered_at timestamptz,
  last_error_code text CHECK (last_error_code IN ('invalid_event', 'processing_failed', 'lease_expired')),
  FOREIGN KEY (workspace_id, audit_event_id, aggregate_version) REFERENCES audit.events(workspace_id, id, resource_version),
  UNIQUE (workspace_id, aggregate_version, event_type),
  UNIQUE (workspace_id, id),
  CHECK ((status = 'PROCESSING') = (lease_token IS NOT NULL AND leased_until IS NOT NULL)),
  CHECK ((lease_token IS NULL) = (leased_until IS NULL)),
  CHECK ((status = 'DELIVERED') = (delivered_at IS NOT NULL))
);
CREATE INDEX outbox_ready ON eventing.outbox(available_at, occurred_at, id) WHERE status IN ('PENDING', 'PROCESSING');
ALTER TABLE eventing.outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE eventing.outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_append ON eventing.outbox FOR INSERT TO foundation_api
WITH CHECK (workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid
  AND status = 'PENDING' AND attempts = 0 AND organizations.authorize_workspace('workspace.update'));
CREATE POLICY outbox_worker ON eventing.outbox FOR ALL TO foundation_worker USING (true) WITH CHECK (true);

-- This private projection is a real asynchronous effect; it stores no workspace name.
-- It proves monotonic delivery without creating an unapproved product feature.
CREATE TABLE eventing.workspace_revisions (
  workspace_id uuid PRIMARY KEY REFERENCES organizations.workspaces(id),
  version bigint NOT NULL CHECK (version > 1),
  last_event_id uuid NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (workspace_id, last_event_id) REFERENCES eventing.outbox(workspace_id, id)
);
CREATE TABLE eventing.consumer_receipts (
  consumer text NOT NULL CHECK (consumer = 'workspace_revision_v1'),
  event_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (consumer, event_id),
  FOREIGN KEY (workspace_id, event_id) REFERENCES eventing.outbox(workspace_id, id)
);
GRANT USAGE ON SCHEMA organizations, audit, eventing TO foundation_api;
GRANT SELECT ON organizations.workspaces TO foundation_api;
GRANT UPDATE (name, version, updated_at) ON organizations.workspaces TO foundation_api;
GRANT INSERT ON audit.events, eventing.outbox TO foundation_api;
GRANT USAGE ON SCHEMA eventing TO foundation_worker;
GRANT SELECT ON eventing.outbox TO foundation_worker;
GRANT UPDATE (status,attempts,available_at,lease_token,leased_until,delivered_at,last_error_code) ON eventing.outbox TO foundation_worker;
GRANT SELECT, INSERT ON eventing.consumer_receipts TO foundation_worker;
GRANT SELECT, INSERT, UPDATE ON eventing.workspace_revisions TO foundation_worker;
