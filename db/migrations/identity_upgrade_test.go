package migrations

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationIdentityUpgradePreservesPackageA(t *testing.T) {
	conn := migrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := catalogue(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(ctx, conn, items[:1]); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO identity.users(id) VALUES('00000000-0000-4000-8000-000000000001');
INSERT INTO organizations.workspaces(id,owner_kind,owner_user_id,name,version) VALUES('10000000-0000-4000-8000-000000000001','PERSON','00000000-0000-4000-8000-000000000001','Existing workspace',2);
INSERT INTO audit.events(id,workspace_id,actor_user_id,action,target_kind,target_id,request_id,correlation_id,result,previous_version,resource_version,metadata)
VALUES('30000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','00000000-0000-4000-8000-000000000001','workspace.name_updated','Workspace','10000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000003','SUCCESS',1,2,'{"previous_version":1,"version":2}');
INSERT INTO eventing.outbox(id,workspace_id,audit_event_id,aggregate_version,event_type,schema_version,payload,correlation_id)
VALUES('50000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001',2,'WorkspaceNameChanged',1,'{}','30000000-0000-4000-8000-000000000003');
INSERT INTO eventing.consumer_receipts(consumer,event_id,workspace_id) VALUES('workspace_revision_v1','50000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001');`)
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = conn.QueryRow(ctx, `SELECT checksum FROM foundation_schema.migrations WHERE version=1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var after, name, actorKind, serviceKind string
	var version, securityVersion int64
	var receiptServiceID string
	err = conn.QueryRow(ctx, `SELECT m.checksum,w.name,w.version,u.security_version,a.actor_kind,r.actor_kind,r.actor_service_id::text
FROM foundation_schema.migrations m CROSS JOIN organizations.workspaces w CROSS JOIN identity.users u CROSS JOIN audit.events a CROSS JOIN eventing.consumer_receipts r WHERE m.version=1`).Scan(&after, &name, &version, &securityVersion, &actorKind, &serviceKind, &receiptServiceID)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || name != "Existing workspace" || version != 2 || securityVersion != 1 || actorKind != "HUMAN" || serviceKind != "SERVICE" || receiptServiceID != "40000000-0000-4000-8000-000000000001" {
		t.Fatal("upgrade changed historical content or lost typed attribution")
	}
	var count int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM eventing.outbox WHERE audit_event_id='30000000-0000-4000-8000-000000000001' AND aggregate_version=2 AND status='PENDING'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("outbox history changed", err)
	}
	if err = Apply(ctx, conn); err != nil {
		t.Fatal("repeat upgrade", err)
	}
}
