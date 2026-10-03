-- P0.4: application-owned identity and sessions. No provider authorization metadata.
ALTER TABLE identity.users ADD COLUMN security_version bigint NOT NULL DEFAULT 1 CHECK (security_version > 0);

CREATE TABLE identity.external_identities (
  issuer text NOT NULL CHECK (char_length(issuer) BETWEEN 8 AND 512 AND issuer LIKE 'https://%'),
  subject text NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 255 AND subject !~ '[[:cntrl:]]'),
  provider text NOT NULL CHECK (provider ~ '^[a-z][a-z0-9_-]{0,63}$'),
  user_id uuid NOT NULL REFERENCES identity.users(id),
  PRIMARY KEY (issuer, subject),
  UNIQUE (issuer,subject,user_id)
);
CREATE TABLE identity.service_principals (
  id uuid PRIMARY KEY,
  name text NOT NULL UNIQUE CHECK (name ~ '^[a-z][a-z0-9_-]{0,63}$')
);
INSERT INTO identity.service_principals(id,name) VALUES ('40000000-0000-4000-8000-000000000001','outbox-worker');
INSERT INTO identity.service_principals(id,name) VALUES ('40000000-0000-4000-8000-000000000002','identity-operator');
ALTER TABLE eventing.consumer_receipts ADD COLUMN actor_kind text NOT NULL DEFAULT 'SERVICE' CHECK (actor_kind='SERVICE');
ALTER TABLE eventing.consumer_receipts ADD COLUMN actor_service_id uuid NOT NULL DEFAULT '40000000-0000-4000-8000-000000000001'
  REFERENCES identity.service_principals(id) CHECK (actor_service_id='40000000-0000-4000-8000-000000000001');

CREATE TABLE identity.login_transactions (
  state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash)=32),
  binding_hash bytea NOT NULL CHECK (octet_length(binding_hash)=32),
  nonce text NOT NULL CHECK (nonce ~ '^[A-Za-z0-9_-]{43}$'),
  pkce_verifier text NOT NULL CHECK (pkce_verifier ~ '^[A-Za-z0-9_-]{43}$'),
  started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '5 minutes',
  request_id uuid NOT NULL,
  correlation_id uuid NOT NULL,
  CHECK (expires_at > started_at AND expires_at <= started_at+interval '5 minutes 1 second')
);
CREATE TABLE identity.login_admission (
  bucket timestamptz PRIMARY KEY,
  attempts integer NOT NULL CHECK (attempts BETWEEN 1 AND 120)
);
CREATE TABLE identity.sessions (
  id uuid PRIMARY KEY,
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
  user_id uuid NOT NULL REFERENCES identity.users(id),
  issuer text NOT NULL,
  subject text NOT NULL,
  security_version bigint NOT NULL CHECK (security_version > 0),
  authenticated_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  last_accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  absolute_expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  assurance_level text NOT NULL CHECK (assurance_level IN ('UNKNOWN','MFA')),
  assurance_evidence text NOT NULL CHECK (char_length(assurance_evidence) <= 120 AND assurance_evidence !~ '[[:cntrl:]]'),
  UNIQUE (user_id,id),
  FOREIGN KEY (issuer,subject,user_id) REFERENCES identity.external_identities(issuer,subject,user_id),
  CHECK (absolute_expires_at=authenticated_at+interval '8 hours'),
  CHECK (authenticated_at<=created_at+interval '60 seconds'),
  CHECK (last_accepted_at>=created_at)
);
CREATE INDEX sessions_user ON identity.sessions(user_id);
CREATE INDEX login_transactions_expiration ON identity.login_transactions(expires_at);

-- Workspace history remains immutable and explicitly HUMAN. Security events are
-- a separate bounded schema because they need no artificial workspace/version.
ALTER TABLE audit.events ADD COLUMN actor_kind text NOT NULL DEFAULT 'HUMAN' CHECK (actor_kind='HUMAN');
CREATE TABLE audit.security_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_kind text NOT NULL CHECK (actor_kind IN ('HUMAN','SERVICE')),
  actor_user_id uuid REFERENCES identity.users(id),
  actor_service_id uuid REFERENCES identity.service_principals(id),
  action text NOT NULL CHECK (action IN ('account.created','session.created','session.revoked','sessions.revoked','account.disabled')),
  target_user_id uuid NOT NULL REFERENCES identity.users(id),
  session_id uuid,
  occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  request_id uuid NOT NULL,
  correlation_id uuid NOT NULL,
  result text NOT NULL DEFAULT 'SUCCESS' CHECK (result='SUCCESS'),
  metadata jsonb NOT NULL DEFAULT '{}' CHECK (metadata='{}'::jsonb),
  FOREIGN KEY (target_user_id,session_id) REFERENCES identity.sessions(user_id,id),
  CHECK ((actor_kind='HUMAN' AND actor_user_id IS NOT NULL AND actor_service_id IS NULL)
      OR (actor_kind='SERVICE' AND actor_service_id IS NOT NULL AND actor_user_id IS NULL)),
  CHECK ((action IN ('session.created','session.revoked'))=(session_id IS NOT NULL)),
  CHECK ((actor_kind='HUMAN' AND actor_user_id=target_user_id AND action<>'account.disabled')
     OR (actor_kind='SERVICE' AND action='account.disabled'))
);

CREATE FUNCTION identity.create_login(p_state bytea,p_binding bytea,p_nonce text,p_verifier text,p_request uuid,p_correlation uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE admitted integer; now_at timestamptz:=clock_timestamp();
BEGIN
  DELETE FROM identity.login_transactions WHERE expires_at<=now_at;
  DELETE FROM identity.login_admission WHERE bucket<date_trunc('minute',now_at);
  INSERT INTO identity.login_admission(bucket,attempts) VALUES(date_trunc('minute',now_at),1)
  ON CONFLICT(bucket) DO UPDATE SET attempts=identity.login_admission.attempts+1 WHERE identity.login_admission.attempts<120
  RETURNING attempts INTO admitted;
  IF admitted IS NULL THEN RETURN false; END IF;
  INSERT INTO identity.login_transactions(state_hash,binding_hash,nonce,pkce_verifier,started_at,expires_at,request_id,correlation_id)
  VALUES(p_state,p_binding,p_nonce,p_verifier,now_at,now_at+interval '5 minutes',p_request,p_correlation);
  RETURN true;
END $$;
CREATE FUNCTION identity.consume_login(p_state bytea,p_binding bytea)
RETURNS TABLE(nonce text,pkce_verifier text,started_at timestamptz,request_id uuid,correlation_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog AS $$
  DELETE FROM identity.login_transactions t WHERE t.state_hash=p_state AND t.binding_hash=p_binding AND t.expires_at>clock_timestamp()
  RETURNING t.nonce,t.pkce_verifier,t.started_at,t.request_id,t.correlation_id
$$;

-- A lock on the account precedes a session lock for all commands. Account-wide
-- revocation takes an exclusive account lock and orders against protected work.
CREATE FUNCTION identity.resolve_session(p_hash bytea)
RETURNS TABLE(user_id uuid,session_id uuid,provider text,issuer text,subject text,authenticated_at timestamptz,assurance_level text,assurance_evidence text,absolute_expires_at timestamptz,idle_expires_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE uid uuid; account identity.users%ROWTYPE; sess identity.sessions%ROWTYPE; provider_name text;
BEGIN
  SELECT s.user_id INTO uid FROM identity.sessions s WHERE s.token_hash=p_hash;
  IF uid IS NULL THEN RETURN; END IF;
  SELECT * INTO account FROM identity.users u WHERE u.id=uid FOR SHARE;
  IF NOT account.active THEN RETURN; END IF;
  SELECT * INTO sess FROM identity.sessions s WHERE s.token_hash=p_hash AND s.user_id=uid FOR UPDATE;
  IF NOT FOUND OR sess.revoked_at IS NOT NULL OR sess.security_version<>account.security_version
     OR sess.absolute_expires_at<=clock_timestamp() OR sess.last_accepted_at+interval '15 minutes'<=clock_timestamp() THEN RETURN; END IF;
  SELECT e.provider INTO provider_name FROM identity.external_identities e WHERE e.issuer=sess.issuer AND e.subject=sess.subject AND e.user_id=uid;
  IF provider_name IS NULL THEN RETURN; END IF;
  RETURN QUERY SELECT uid,sess.id,provider_name,sess.issuer,sess.subject,sess.authenticated_at,sess.assurance_level,sess.assurance_evidence,sess.absolute_expires_at,sess.last_accepted_at+interval '15 minutes';
END $$;

CREATE FUNCTION identity.complete_login(p_provider text,p_issuer text,p_subject text,p_verified boolean,p_auth_time timestamptz,p_assurance text,p_evidence text,p_hash bytea,p_previous bytea,p_started_at timestamptz,p_request uuid,p_correlation uuid)
RETURNS bytea LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE uid uuid; previous_uid uuid; previous_id uuid; version_now bigint; active_now boolean; sid uuid:=gen_random_uuid(); now_at timestamptz:=clock_timestamp();
BEGIN
  IF p_verified IS DISTINCT FROM true OR p_auth_time IS NULL OR p_started_at IS NULL
    OR p_started_at<now_at-interval '5 minutes' OR p_started_at>now_at
    OR p_auth_time<p_started_at-interval '60 seconds' OR p_auth_time>now_at+interval '60 seconds' THEN RETURN NULL; END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_issuer||chr(31)||p_subject,1704));
  SELECT e.user_id INTO uid FROM identity.external_identities e WHERE e.issuer=p_issuer AND e.subject=p_subject;
  IF uid IS NULL THEN
    uid:=gen_random_uuid();
    INSERT INTO identity.users(id) VALUES(uid);
    INSERT INTO identity.external_identities(issuer,subject,provider,user_id) VALUES(p_issuer,p_subject,p_provider,uid);
    INSERT INTO audit.security_events(actor_kind,actor_user_id,action,target_user_id,request_id,correlation_id)
      VALUES('HUMAN',uid,'account.created',uid,p_request,p_correlation);
  ELSE
    IF NOT EXISTS(SELECT FROM identity.external_identities e WHERE e.issuer=p_issuer AND e.subject=p_subject AND e.provider=p_provider) THEN RETURN NULL; END IF;
  END IF;
  SELECT s.user_id,s.id INTO previous_uid,previous_id FROM identity.sessions s WHERE s.token_hash=p_previous;
  IF previous_uid IS NOT NULL AND previous_uid<>uid THEN RETURN NULL; END IF;
  PERFORM u.id FROM identity.users u WHERE u.id IN (uid,previous_uid) ORDER BY u.id FOR UPDATE;
  SELECT u.active,u.security_version INTO active_now,version_now FROM identity.users u WHERE u.id=uid;
  IF NOT active_now THEN RETURN NULL; END IF;
  now_at:=clock_timestamp();
  IF p_started_at<now_at-interval '5 minutes' THEN RETURN NULL; END IF;
  IF previous_id IS NOT NULL THEN
    UPDATE identity.sessions SET revoked_at=now_at WHERE id=previous_id AND revoked_at IS NULL;
    IF FOUND THEN
      INSERT INTO audit.security_events(actor_kind,actor_user_id,action,target_user_id,session_id,request_id,correlation_id)
        VALUES('HUMAN',previous_uid,'session.revoked',previous_uid,previous_id,p_request,p_correlation);
    END IF;
  END IF;
  INSERT INTO identity.sessions(id,token_hash,user_id,issuer,subject,security_version,authenticated_at,created_at,last_accepted_at,absolute_expires_at,assurance_level,assurance_evidence)
    VALUES(sid,p_hash,uid,p_issuer,p_subject,version_now,p_auth_time,now_at,now_at,p_auth_time+interval '8 hours',p_assurance,p_evidence);
  INSERT INTO audit.security_events(actor_kind,actor_user_id,action,target_user_id,session_id,request_id,correlation_id)
    VALUES('HUMAN',uid,'session.created',uid,sid,p_request,p_correlation);
  RETURN p_hash;
END $$;

CREATE FUNCTION identity.accept_session(p_hash bytea) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE sid uuid;
BEGIN
  SELECT r.session_id INTO sid FROM identity.resolve_session(p_hash) r;
  IF sid IS NULL THEN RETURN false; END IF;
  UPDATE identity.sessions SET last_accepted_at=clock_timestamp() WHERE id=sid;
  RETURN true;
END $$;

CREATE FUNCTION identity.revoke_session(p_hash bytea,p_all boolean,p_request uuid,p_correlation uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE uid uuid; sid uuid;
BEGIN
  SELECT s.user_id INTO uid FROM identity.sessions s WHERE s.token_hash=p_hash;
  IF uid IS NULL THEN RETURN false; END IF;
  PERFORM u.id FROM identity.users u WHERE u.id=uid FOR UPDATE;
  SELECT r.session_id INTO sid FROM identity.resolve_session(p_hash) r;
  IF sid IS NULL THEN RETURN false; END IF;
  IF p_all THEN
    UPDATE identity.users SET security_version=security_version+1 WHERE id=uid;
    UPDATE identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=uid AND revoked_at IS NULL;
    INSERT INTO audit.security_events(actor_kind,actor_user_id,action,target_user_id,request_id,correlation_id)
      VALUES('HUMAN',uid,'sessions.revoked',uid,p_request,p_correlation);
  ELSE
    UPDATE identity.sessions SET revoked_at=clock_timestamp() WHERE id=sid;
    INSERT INTO audit.security_events(actor_kind,actor_user_id,action,target_user_id,session_id,request_id,correlation_id)
      VALUES('HUMAN',uid,'session.revoked',uid,sid,p_request,p_correlation);
  END IF;
  RETURN true;
END $$;

-- No runtime role may disable accounts. This operator primitive remains owned
-- by the migration identity; a future administrative capability requires review.
CREATE FUNCTION identity.disable_account(p_user uuid,p_request uuid,p_correlation uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
  UPDATE identity.users SET active=false,security_version=security_version+1 WHERE id=p_user AND active;
  IF NOT FOUND THEN RETURN false; END IF;
  UPDATE identity.sessions SET revoked_at=clock_timestamp() WHERE user_id=p_user AND revoked_at IS NULL;
  INSERT INTO audit.security_events(actor_kind,actor_service_id,action,target_user_id,request_id,correlation_id)
    VALUES('SERVICE','40000000-0000-4000-8000-000000000002','account.disabled',p_user,p_request,p_correlation);
  RETURN true;
END $$;

CREATE OR REPLACE FUNCTION organizations.authorize_workspace(required_permission text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE allowed boolean;
BEGIN
  SELECT true INTO allowed
  FROM identity.users u
  JOIN identity.sessions s ON s.user_id=u.id
  JOIN organizations.workspace_memberships m ON m.user_id=u.id
  JOIN organizations.workspace_permissions p ON p.workspace_id=m.workspace_id AND p.user_id=m.user_id
  WHERE u.id=nullif(current_setting('app.actor_id',true),'')::uuid
    AND s.token_hash=decode(nullif(current_setting('app.session_hash',true),''),'hex')
    AND m.workspace_id=nullif(current_setting('app.workspace_id',true),'')::uuid
    AND u.active AND m.active AND p.permission=required_permission
    AND s.security_version=u.security_version AND s.revoked_at IS NULL
    AND s.absolute_expires_at>clock_timestamp() AND s.last_accepted_at+interval '15 minutes'>clock_timestamp()
  FOR SHARE OF u,s,m,p;
  RETURN coalesce(allowed,false);
END $$;

REVOKE ALL ON ALL FUNCTIONS IN SCHEMA identity FROM PUBLIC;
GRANT USAGE ON SCHEMA identity TO foundation_api;
GRANT EXECUTE ON FUNCTION identity.create_login(bytea,bytea,text,text,uuid,uuid),identity.consume_login(bytea,bytea),
  identity.complete_login(text,text,text,boolean,timestamptz,text,text,bytea,bytea,timestamptz,uuid,uuid),identity.resolve_session(bytea),
  identity.accept_session(bytea),identity.revoke_session(bytea,boolean,uuid,uuid) TO foundation_api;
