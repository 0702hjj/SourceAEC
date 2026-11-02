// SPDX-License-Identifier: Apache-2.0
package workflow

import "context"

func Schema(ctx context.Context, db SQL) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_definitions (
 model_id TEXT NOT NULL CHECK(model_id ~ '^m_[0-9a-f]{16}$'),
 digest TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('ifc','dxf')),
 body BYTEA NOT NULL, input_bytes BIGINT NOT NULL CHECK(input_bytes BETWEEN 0 AND 271581184),
 PRIMARY KEY(model_id,digest), CHECK(encode(sha256(body),'hex')=digest));
 CREATE TABLE IF NOT EXISTS workflow_runs (
 id TEXT PRIMARY KEY CHECK(id ~ '^wf_[0-9a-f]{32}$'), model_id TEXT NOT NULL,
 definition TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('ifc','dxf')),
 action TEXT NOT NULL CHECK(action IN ('run','save')), request_key TEXT NOT NULL,
 intent BYTEA NOT NULL CHECK(octet_length(intent)<=1048576), intent_digest TEXT NOT NULL,
 base_identity TEXT NOT NULL CHECK(base_identity ~ '^[0-9a-f]{64}$'),
 input_bytes BIGINT NOT NULL CHECK(input_bytes BETWEEN 0 AND 271581184),
 phase TEXT NOT NULL CHECK(phase IN ('geometry','publication','conversion')),
 state TEXT NOT NULL CHECK(state IN ('pending','running','succeeded','failed','canceled','superseded')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0),
 phase_attempts INTEGER NOT NULL DEFAULT 0 CHECK(phase_attempts BETWEEN 0 AND 3),
 lease_owner TEXT, lease_token TEXT, lease_until TIMESTAMPTZ,
 candidate BYTEA, result BYTEA CHECK(octet_length(result)<=1048576),
 error TEXT NOT NULL DEFAULT '' CHECK(octet_length(error)<=2048),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 available_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(model_id,request_key), FOREIGN KEY(model_id,definition) REFERENCES workflow_definitions(model_id,digest) ON DELETE CASCADE,
 CHECK(encode(sha256(intent),'hex')=intent_digest),
 CHECK((state='running')=(lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_until IS NOT NULL)),
 CHECK(state='running' OR (lease_owner IS NULL AND lease_token IS NULL AND lease_until IS NULL)),
 CHECK(phase='geometry' OR candidate IS NOT NULL), CHECK(phase!='conversion' OR result IS NOT NULL),
 CHECK(state!='succeeded' OR (result IS NOT NULL AND phase IN ('publication','conversion'))));
 CREATE INDEX IF NOT EXISTS workflow_runnable ON workflow_runs(created_at,id) WHERE state IN ('pending','running');
 CREATE INDEX IF NOT EXISTS workflow_model ON workflow_runs(model_id,created_at DESC,id);
 CREATE TABLE IF NOT EXISTS workflow_attempts (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
 attempt INTEGER NOT NULL, phase TEXT NOT NULL, owner TEXT NOT NULL, token TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('running','completed','failed','abandoned','canceled','superseded')),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), finished_at TIMESTAMPTZ,
 error TEXT NOT NULL DEFAULT '' CHECK(octet_length(error)<=2048), PRIMARY KEY(run_id,attempt),
 CHECK((state='running')=(finished_at IS NULL)));`)
	return err
}
