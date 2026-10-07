-- Approved private Business -> Listing Draft -> synchronous protected Preview.
-- Existing Workspace audit/outbox contracts remain unchanged. No draft event
-- is emitted because this slice has no asynchronous responsibility.
CREATE SCHEMA listings;
REVOKE ALL ON SCHEMA listings FROM PUBLIC;
GRANT USAGE ON SCHEMA listings TO foundation_api;

ALTER TABLE organizations.workspace_permissions DROP CONSTRAINT workspace_permissions_permission_check;
ALTER TABLE organizations.workspace_permissions ADD CONSTRAINT workspace_permissions_permission_check
  CHECK (permission IN ('workspace.read','workspace.update','listing.read_private','listing.create','listing.update'));

-- This identifies a default drafting context, not a limit of one personally
-- owned workspace. Composite ownership prevents assigning someone else's tenant.
ALTER TABLE organizations.workspaces ADD CONSTRAINT workspace_personal_owner_identity UNIQUE (owner_user_id,id);
CREATE TABLE organizations.personal_drafting_assignments (
  user_id uuid PRIMARY KEY REFERENCES identity.users(id),
  workspace_id uuid NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (user_id,workspace_id) REFERENCES organizations.workspaces(owner_user_id,id)
);

CREATE TABLE listings.businesses (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  private_label text NOT NULL DEFAULT '' CHECK (char_length(private_label)<=120 AND private_label=btrim(private_label) AND private_label !~ E'[[:cntrl:]\u0080-\u009f\u2028\u2029]'),
  activity_description text NOT NULL DEFAULT '' CHECK (char_length(activity_description)<=2000 AND activity_description=btrim(activity_description) AND regexp_replace(activity_description,E'[\n\t]','','g') !~ E'[[:cntrl:]\u0080-\u009f\u2028\u2029]'),
  country text NOT NULL DEFAULT '' CHECK (country='' OR country ~ '^[A-Z]{2}$'),
  region text NOT NULL DEFAULT '' CHECK (char_length(region)<=120 AND region=btrim(region) AND region !~ E'[[:cntrl:]\u0080-\u009f\u2028\u2029]'),
  version bigint NOT NULL DEFAULT 1 CHECK (version>0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (workspace_id,id)
);
CREATE TABLE listings.drafts (
  id uuid PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  business_id uuid NOT NULL,
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status='DRAFT'),
  title text NOT NULL DEFAULT '' CHECK (char_length(title)<=160 AND title=btrim(title) AND title !~ E'[[:cntrl:]\u0080-\u009f\u2028\u2029]'),
  description text NOT NULL DEFAULT '' CHECK (char_length(description)<=8000 AND description=btrim(description) AND regexp_replace(description,E'[\n\t]','','g') !~ E'[[:cntrl:]\u0080-\u009f\u2028\u2029]'),
  sale_subject text NOT NULL DEFAULT '' CHECK (sale_subject IN ('','legal_entity','operating_business','business_division','asset_package')),
  transfer_structure text NOT NULL DEFAULT '' CHECK (transfer_structure IN ('','share_sale','business_transfer')),
  sale_context text NOT NULL DEFAULT '' CHECK (sale_context IN ('','ordinary_sale','succession','restructuring','insolvency','other')),
  sale_method text NOT NULL DEFAULT '' CHECK (sale_method IN ('','asking_price','negotiation','invitation_for_offers','time_limited_bidding')),
  version bigint NOT NULL DEFAULT 1 CHECK (version>0),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY (workspace_id,business_id) REFERENCES listings.businesses(workspace_id,id),
  UNIQUE (workspace_id,id),
  UNIQUE (workspace_id,business_id,id)
);

-- Finite typed targets have real ownership FKs, rather than an unchecked
-- polymorphic identifier. Actor attribution is bound to the admitted session.
CREATE TABLE audit.aggregate_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  actor_kind text NOT NULL DEFAULT 'HUMAN' CHECK (actor_kind='HUMAN'),
  actor_user_id uuid NOT NULL REFERENCES identity.users(id),
  actor_session_id uuid NOT NULL,
  action text NOT NULL CHECK (action IN ('workspace.personal_drafting_initialized','business.created','business.updated','listing_draft.created','listing_draft.updated')),
  target_kind text NOT NULL CHECK (target_kind IN ('Workspace','Business','ListingDraft')),
  workspace_target_id uuid,
  business_id uuid,
  listing_id uuid,
  target_id uuid GENERATED ALWAYS AS (coalesce(workspace_target_id,business_id,listing_id)) STORED,
  occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  request_id uuid NOT NULL,
  correlation_id uuid NOT NULL,
  result text NOT NULL DEFAULT 'SUCCESS' CHECK (result='SUCCESS'),
  previous_version bigint NOT NULL CHECK (previous_version>=0),
  resource_version bigint NOT NULL CHECK (resource_version=previous_version+1),
  metadata jsonb NOT NULL,
  FOREIGN KEY (actor_user_id,actor_session_id) REFERENCES identity.sessions(user_id,id),
  FOREIGN KEY (workspace_target_id) REFERENCES organizations.workspaces(id),
  FOREIGN KEY (workspace_id,business_id) REFERENCES listings.businesses(workspace_id,id),
  FOREIGN KEY (workspace_id,listing_id) REFERENCES listings.drafts(workspace_id,id),
  CHECK (metadata=jsonb_build_object('previous_version',previous_version,'version',resource_version)),
  CHECK ((target_kind='Workspace' AND workspace_target_id IS NOT NULL AND workspace_target_id=workspace_id AND business_id IS NULL AND listing_id IS NULL AND action='workspace.personal_drafting_initialized' AND previous_version=0)
      OR (target_kind='Business' AND business_id IS NOT NULL AND workspace_target_id IS NULL AND listing_id IS NULL AND ((action='business.created' AND previous_version=0) OR (action='business.updated' AND previous_version>0)))
      OR (target_kind='ListingDraft' AND listing_id IS NOT NULL AND workspace_target_id IS NULL AND business_id IS NULL AND ((action='listing_draft.created' AND previous_version=0) OR (action='listing_draft.updated' AND previous_version>0)))),
  UNIQUE (workspace_id,target_kind,target_id,resource_version)
);
CREATE INDEX aggregate_events_actor ON audit.aggregate_events(actor_user_id,occurred_at,id);

CREATE TABLE listings.command_receipts (
  actor_user_id uuid NOT NULL REFERENCES identity.users(id),
  context_kind text NOT NULL CHECK (context_kind IN ('WORKSPACE','PERSONAL')),
  workspace_id uuid NOT NULL REFERENCES organizations.workspaces(id),
  operation text NOT NULL CHECK (operation='listing_draft.create'),
  command_id uuid NOT NULL,
  request_hash bytea NOT NULL CHECK (octet_length(request_hash)=32),
  business_id uuid NOT NULL,
  listing_id uuid NOT NULL,
  business_version bigint NOT NULL CHECK (business_version=1),
  listing_version bigint NOT NULL CHECK (listing_version=1),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (actor_user_id,context_kind,workspace_id,operation,command_id),
  FOREIGN KEY (workspace_id,business_id,listing_id) REFERENCES listings.drafts(workspace_id,business_id,id)
);

CREATE FUNCTION listings.authorize_context(required_permission text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$
  SELECT organizations.authorize_workspace('listing.read_private')
    AND (required_permission='listing.read_private' OR organizations.authorize_workspace(required_permission))
$$;
REVOKE ALL ON FUNCTION listings.authorize_context(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION listings.authorize_context(text) TO foundation_api;

ALTER TABLE listings.businesses ENABLE ROW LEVEL SECURITY;
ALTER TABLE listings.businesses FORCE ROW LEVEL SECURITY;
ALTER TABLE listings.drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE listings.drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY business_read ON listings.businesses FOR SELECT TO foundation_api
USING (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.read_private'));
CREATE POLICY business_create ON listings.businesses FOR INSERT TO foundation_api
WITH CHECK (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.create'));
CREATE POLICY business_update ON listings.businesses FOR UPDATE TO foundation_api
USING (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.update'))
WITH CHECK (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.update'));
CREATE POLICY draft_read ON listings.drafts FOR SELECT TO foundation_api
USING (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.read_private'));
CREATE POLICY draft_create ON listings.drafts FOR INSERT TO foundation_api
WITH CHECK (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.create'));
CREATE POLICY draft_update ON listings.drafts FOR UPDATE TO foundation_api
USING (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.update'))
WITH CHECK (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.update'));

ALTER TABLE listings.command_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE listings.command_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY receipt_read ON listings.command_receipts FOR SELECT TO foundation_api
USING (actor_user_id=nullif(current_setting('app.actor_id',true),'')::uuid AND workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.create'));
CREATE POLICY receipt_append ON listings.command_receipts FOR INSERT TO foundation_api
WITH CHECK (actor_user_id=nullif(current_setting('app.actor_id',true),'')::uuid AND workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid AND listings.authorize_context('listing.create'));
ALTER TABLE audit.aggregate_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit.aggregate_events FORCE ROW LEVEL SECURITY;
CREATE POLICY aggregate_append ON audit.aggregate_events FOR INSERT TO foundation_api
WITH CHECK (workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid
  AND actor_user_id=nullif(current_setting('app.actor_id',true),'')::uuid
  AND actor_session_id=(SELECT r.session_id FROM identity.resolve_session(decode(nullif(current_setting('app.session_hash',true),''),'hex')) r)
  AND ((action IN ('business.created','listing_draft.created') AND listings.authorize_context('listing.create'))
    OR (action IN ('business.updated','listing_draft.updated') AND listings.authorize_context('listing.update'))));

-- Definers are owned by the migration identity, whose login is rejected for
-- runtime use. Explicit owner policies make these narrow helpers work under
-- FORCE RLS even when that identity is not a superuser and has no BYPASSRLS.
CREATE POLICY aggregate_helper_read ON audit.aggregate_events FOR SELECT TO CURRENT_USER USING (true);
CREATE POLICY business_helper_read ON listings.businesses FOR SELECT TO CURRENT_USER USING (true);
CREATE POLICY draft_helper_read ON listings.drafts FOR SELECT TO CURRENT_USER USING (true);
CREATE POLICY workspace_helper_read ON organizations.workspaces FOR SELECT TO CURRENT_USER USING (true);
CREATE POLICY workspace_drafting_bootstrap ON organizations.workspaces FOR INSERT TO CURRENT_USER
WITH CHECK (owner_kind='PERSON' AND owner_user_id=(SELECT r.user_id FROM identity.resolve_session(decode(nullif(current_setting('app.session_hash',true),''),'hex')) r));
CREATE POLICY aggregate_drafting_bootstrap ON audit.aggregate_events FOR INSERT TO CURRENT_USER
WITH CHECK (action='workspace.personal_drafting_initialized'
  AND actor_user_id=nullif(current_setting('app.actor_id',true),'')::uuid
  AND actor_session_id=(SELECT r.session_id FROM identity.resolve_session(decode(nullif(current_setting('app.session_hash',true),''),'hex')) r));

-- The reverse binding also rejects fabricated future evidence for a resource
-- which merely exists: an inserted event must describe its current revision.
CREATE FUNCTION listings.enforce_audit_revision() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE actual bigint;
BEGIN
  CASE NEW.target_kind
    WHEN 'Business' THEN SELECT b.version INTO actual FROM listings.businesses b WHERE b.workspace_id=NEW.workspace_id AND b.id=NEW.business_id;
    WHEN 'ListingDraft' THEN SELECT d.version INTO actual FROM listings.drafts d WHERE d.workspace_id=NEW.workspace_id AND d.id=NEW.listing_id;
    WHEN 'Workspace' THEN SELECT w.version INTO actual FROM organizations.workspaces w WHERE w.id=NEW.workspace_target_id;
  END CASE;
  IF actual IS DISTINCT FROM NEW.resource_version THEN
    RAISE EXCEPTION 'audit must describe current resource revision' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION listings.enforce_audit_revision() FROM PUBLIC;
CREATE TRIGGER aggregate_target_revision BEFORE INSERT ON audit.aggregate_events
FOR EACH ROW EXECUTE FUNCTION listings.enforce_audit_revision();

-- Database enforcement prevents committing a revision without its atomic audit,
-- including direct SQL using the restricted application role.
CREATE FUNCTION listings.enforce_revision_audit() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE prior bigint:=0; kind text; event_action text;
BEGIN
  kind:=CASE TG_TABLE_NAME WHEN 'businesses' THEN 'Business' ELSE 'ListingDraft' END;
  event_action:=CASE TG_TABLE_NAME WHEN 'businesses' THEN 'business.' ELSE 'listing_draft.' END;
  IF TG_OP='UPDATE' THEN prior:=OLD.version; END IF;
  event_action:=event_action||CASE TG_OP WHEN 'INSERT' THEN 'created' ELSE 'updated' END;
  IF NEW.version<>prior+1 OR NOT EXISTS(SELECT FROM audit.aggregate_events a
    WHERE a.workspace_id=NEW.workspace_id AND a.target_kind=kind AND a.target_id=NEW.id
      AND a.action=event_action AND a.previous_version=prior AND a.resource_version=NEW.version) THEN
    RAISE EXCEPTION 'revision requires atomic audit' USING ERRCODE='23514';
  END IF;
  RETURN NULL;
END $$;
REVOKE ALL ON FUNCTION listings.enforce_revision_audit() FROM PUBLIC;
CREATE CONSTRAINT TRIGGER business_revision_audit AFTER INSERT OR UPDATE ON listings.businesses
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION listings.enforce_revision_audit();
CREATE CONSTRAINT TRIGGER draft_revision_audit AFTER INSERT OR UPDATE ON listings.drafts
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION listings.enforce_revision_audit();

-- The organizations capability owns bootstrap writes. Acquire the account
-- exclusive lock BEFORE the session lock; do not upgrade a SHARE lock after two
-- independent sessions enter the workflow. Reuse never repairs removed grants
-- or reactivates membership. Revocation remains authoritative.
CREATE FUNCTION organizations.personal_drafting_workspace(p_hash bytea,p_request uuid,p_correlation uuid) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE uid uuid; sid uuid; wid uuid;
BEGIN
  SELECT s.user_id INTO uid FROM identity.sessions s WHERE s.token_hash=p_hash;
  IF uid IS NULL THEN RETURN NULL; END IF;
  PERFORM u.id FROM identity.users u WHERE u.id=uid FOR UPDATE;
  SELECT r.session_id INTO sid FROM identity.resolve_session(p_hash) r WHERE r.user_id=uid;
  IF sid IS NULL THEN RETURN NULL; END IF;
  SELECT a.workspace_id INTO wid FROM organizations.personal_drafting_assignments a WHERE a.user_id=uid;
  PERFORM set_config('app.actor_id',uid::text,true),set_config('app.session_hash',encode(p_hash,'hex'),true);
  IF wid IS NULL THEN
    wid:=gen_random_uuid();
    INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,name) VALUES(wid,'PERSON',uid,'Personal drafting workspace');
    INSERT INTO organizations.workspace_memberships(workspace_id,user_id) VALUES(wid,uid);
    INSERT INTO organizations.workspace_permissions(workspace_id,user_id,permission)
      SELECT wid,uid,permission FROM unnest(ARRAY['workspace.read','workspace.update','listing.read_private','listing.create','listing.update']) permission;
    INSERT INTO organizations.personal_drafting_assignments(user_id,workspace_id) VALUES(uid,wid);
    INSERT INTO audit.aggregate_events(workspace_id,actor_user_id,actor_session_id,action,target_kind,workspace_target_id,request_id,correlation_id,previous_version,resource_version,metadata)
      VALUES(wid,uid,sid,'workspace.personal_drafting_initialized','Workspace',wid,p_request,p_correlation,0,1,'{"previous_version":0,"version":1}');
  END IF;
  PERFORM set_config('app.actor_id',uid::text,true),set_config('app.workspace_id',wid::text,true),set_config('app.session_hash',encode(p_hash,'hex'),true);
  RETURN wid;
END $$;
REVOKE ALL ON FUNCTION organizations.personal_drafting_workspace(bytea,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION organizations.personal_drafting_workspace(bytea,uuid,uuid) TO foundation_api;

GRANT SELECT ON listings.businesses,listings.drafts,listings.command_receipts TO foundation_api;
GRANT INSERT (id,workspace_id,private_label,activity_description,country,region) ON listings.businesses TO foundation_api;
GRANT INSERT (id,workspace_id,business_id,title,description,sale_subject,transfer_structure,sale_context,sale_method) ON listings.drafts TO foundation_api;
GRANT INSERT (actor_user_id,context_kind,workspace_id,operation,command_id,request_hash,business_id,listing_id,business_version,listing_version) ON listings.command_receipts TO foundation_api;
GRANT UPDATE (private_label,activity_description,country,region,version,updated_at) ON listings.businesses TO foundation_api;
GRANT UPDATE (title,description,sale_subject,transfer_structure,sale_context,sale_method,version,updated_at) ON listings.drafts TO foundation_api;
GRANT INSERT (workspace_id,actor_user_id,actor_session_id,action,target_kind,business_id,listing_id,request_id,correlation_id,previous_version,resource_version,metadata) ON audit.aggregate_events TO foundation_api;
-- No DELETE, ledger UPDATE, identity/grant table writes or worker capability.
