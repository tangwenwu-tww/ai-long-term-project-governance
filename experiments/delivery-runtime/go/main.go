package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

const eventPreimage = `{"actor_run_id":"run-executor-1","event_id":"evt-000001","event_type":"EXECUTION_STARTED","from":"PLANNED","instance_id":"inst-demo","prev_event_sha256":"0000000000000000000000000000000000000000000000000000000000000000","schema_version":"0.1-draft","sequence":1,"task_id":"T-042","timestamp":"2026-11-24T02:00:00Z","to":"EXECUTING"}`
const expectedHash = "81588443ac9948f2eda8f0269294b006d10be5d8f4e7dcf4bb23ef17a88d624d"
const initPayload = `{"action":"trust_init","approved_by":"human:project-owner","bootstrap_evidence_ref":"ceremony:demo-001","instance_id":"inst-demo","issued_at":"2026-11-24T02:00:00Z","nonce":"AAAAAAAAAAAAAAAAAAAAAA","owner_key":{"crv":"Ed25519","kid":"owner-v1","kty":"OKP","x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"},"recovery_key":{"crv":"Ed25519","kid":"recovery-v1","kty":"OKP","x":"PUAXw-hDiVqStwqnTRt-vJyYLM8uxJaMwM1V8Sr0Zgw"}}`
const ownerPublicKey = "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
const ownerSignature = "jc7nBV-DsARSLvnRZy0sbN04KtPgZ24m3JfW3bkpry-DGHAywyUKG_eC2FtJWRdOFPzDx_8O417cmfCIj9HnCg"

type result struct {
	Runtime          string `json:"runtime"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	EventSHA256      string `json:"event_sha256"`
	HashMatches      bool   `json:"hash_matches"`
	OwnerSignatureOK bool   `json:"owner_signature_ok"`
}

func decodeBase64URL(value string) []byte {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return decoded
}

func main() {
	digest := sha256.Sum256([]byte(eventPreimage))
	digestHex := hex.EncodeToString(digest[:])
	verified := ed25519.Verify(
		ed25519.PublicKey(decodeBase64URL(ownerPublicKey)),
		[]byte(initPayload),
		decodeBase64URL(ownerSignature),
	)
	output := result{
		Runtime:          runtime.Version(),
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		EventSHA256:      digestHex,
		HashMatches:      digestHex == expectedHash,
		OwnerSignatureOK: verified,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(string(encoded))
	if !output.HashMatches || !output.OwnerSignatureOK {
		os.Exit(1)
	}
}
