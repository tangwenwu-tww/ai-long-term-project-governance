'use strict';

const crypto = require('node:crypto');

const eventPreimage = '{"actor_run_id":"run-executor-1","event_id":"evt-000001","event_type":"EXECUTION_STARTED","from":"PLANNED","instance_id":"inst-demo","prev_event_sha256":"0000000000000000000000000000000000000000000000000000000000000000","schema_version":"0.1-draft","sequence":1,"task_id":"T-042","timestamp":"2026-11-24T02:00:00Z","to":"EXECUTING"}';
const expectedHash = '81588443ac9948f2eda8f0269294b006d10be5d8f4e7dcf4bb23ef17a88d624d';
const initPayload = '{"action":"trust_init","approved_by":"human:project-owner","bootstrap_evidence_ref":"ceremony:demo-001","instance_id":"inst-demo","issued_at":"2026-11-24T02:00:00Z","nonce":"AAAAAAAAAAAAAAAAAAAAAA","owner_key":{"crv":"Ed25519","kid":"owner-v1","kty":"OKP","x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"},"recovery_key":{"crv":"Ed25519","kid":"recovery-v1","kty":"OKP","x":"PUAXw-hDiVqStwqnTRt-vJyYLM8uxJaMwM1V8Sr0Zgw"}}';
const ownerSignature = 'jc7nBV-DsARSLvnRZy0sbN04KtPgZ24m3JfW3bkpry-DGHAywyUKG_eC2FtJWRdOFPzDx_8O417cmfCIj9HnCg';
const ownerJWK = {
  crv: 'Ed25519',
  kid: 'owner-v1',
  kty: 'OKP',
  x: '11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo',
};

const digest = crypto.createHash('sha256').update(eventPreimage, 'utf8').digest('hex');
const publicKey = crypto.createPublicKey({ key: ownerJWK, format: 'jwk' });
const verified = crypto.verify(
  null,
  Buffer.from(initPayload, 'utf8'),
  publicKey,
  Buffer.from(ownerSignature, 'base64url'),
);

const output = {
  runtime: process.version,
  platform: process.platform,
  arch: process.arch,
  event_sha256: digest,
  hash_matches: digest === expectedHash,
  owner_signature_ok: verified,
};
console.log(JSON.stringify(output));
if (!output.hash_matches || !output.owner_signature_ok) {
  process.exitCode = 1;
}
