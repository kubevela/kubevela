# KEP-2.5: Credential Model & Token Rotation

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Communication between hub and spoke is entirely hub-initiated. The hub reads and writes to spoke clusters via cluster-gateway; the spoke never opens a connection to the hub. The spoke "responds" to hub requests by writing to its own local API, which the hub reads back through cluster-gateway. No persistent outbound connection from spoke to hub is required.

| Direction | Mechanism |
|---|---|
| Hub → spoke | cluster-gateway (kubectl proxy) |
| Spoke → hub | Spoke writes to local API; hub reads via cluster-gateway |

## Bootstrap — Spoke Registration

When a spoke is registered from the hub (using a kubeconfig provided by the operator):

1. Hub generates an **ECDH keypair** (P-256) for the spoke relationship
2. Both sides perform a key agreement to derive a **shared AES-256-GCM symmetric key** — neither side transmits this secret; both independently compute the same value from the keypair material
3. Hub stores the derived symmetric key and the spoke's public key in `vela-system/spoke-credential-<spoke-name>` on the hub
4. Hub writes its own public key to the spoke via the bootstrap kubeconfig as `vela-system/hub-credential`
5. Spoke derives the same shared symmetric key locally and stores it alongside the hub public key
6. The bootstrap kubeconfig is discarded after registration — all subsequent communication uses cluster-gateway with the registered credential

The shared AES-256-GCM symmetric key is the **long-lived bootstrap credential**. It persists across token rotation cycles and is only refreshed via an explicit re-registration flow (see Bootstrap Credential Refresh below).

## Token Rotation Protocol

The hub drives all token rotation. A request/response Secret on the spoke serves as the message bus — the hub writes a request, the spoke updates it with a response, the hub reads it back via cluster-gateway.

**Step 1 — Hub initiates:**
1. Hub generates a fresh **RSA-OAEP keypair** (2048-bit) for this rotation cycle
2. Hub constructs: `{ newPublicKey: <RSA public key>, requestId: <uuid>, issuedAt: <timestamp> }`
3. Hub encrypts the payload with the **shared AES-256-GCM symmetric key**
4. Hub writes the encrypted payload to the spoke: `vela-system/token-rotation-<requestId>` labelled `oam.dev/hub-request: token-rotation`

**Step 2 — Spoke processes:**
1. Spoke watches for Secrets labelled `oam.dev/hub-request: token-rotation` on its local API
2. Spoke decrypts the payload using its copy of the shared AES-256-GCM symmetric key
3. Spoke generates or rotates the cluster-gateway access token
4. Spoke encrypts the new token with the hub's **new RSA public key** from the payload — the token is now only readable by the holder of the matching private key
5. Spoke writes `{ encryptedToken: <RSA-encrypted token>, requestId: <uuid>, respondedAt: <timestamp> }` back to the same Secret

**Step 3 — Hub collects:**
1. Hub watches the request Secret via cluster-gateway, detects response fields present
2. Hub decrypts the token with its **new RSA private key**
3. Hub stores the new token in `vela-system/spoke-credential-<spoke-name>`
4. Hub deletes the request Secret from the spoke
5. Hub logs the rotation event for audit

The fresh RSA keypair per rotation cycle provides **forward secrecy on token delivery** — compromise of one cycle's private key cannot expose tokens from any other cycle.

## Secret Lifecycle & Audit

Request Secrets are deleted from the spoke immediately after the hub reads and stores the response. The hub-side audit trail is maintained via structured controller logs:

```
INFO  token-rotation  spoke=prod-eu requestId=abc123 status=initiated
INFO  token-rotation  spoke=prod-eu requestId=abc123 status=response-received respondedAt=...
INFO  token-rotation  spoke=prod-eu requestId=abc123 status=complete secret-deleted=true
```

## Bootstrap Credential Refresh

If the long-lived shared symmetric key needs manual rotation the operator triggers a re-registration flow — providing a fresh kubeconfig, repeating the ECDH key agreement, and overwriting the symmetric key on both sides. Existing token rotation resumes using the new symmetric key on the next scheduled cycle.

## Cryptographic Primitives

| Purpose | Algorithm | Rationale |
|---|---|---|
| Bootstrap key agreement | ECDH (P-256) | Standard, no key transmission required |
| Request/response envelope | AES-256-GCM | Fast authenticated symmetric encryption |
| Token delivery | RSA-OAEP (2048-bit) | Token readable only by hub even if symmetric key is later compromised |
| Key material storage | Kubernetes Secret | Standard; can be KMS-backed (see Open Questions) |

## Dispatch Integrity

The shared AES-256-GCM symmetric key doubles as a dispatch integrity mechanism. Every Component CR dispatched by the hub carries an HMAC-SHA256 signature annotation computed over the Component's key fields. The spoke component-controller validates this signature before processing — rejecting any Component that cannot be authenticated as originating from the hub.

The annotation:

```yaml
metadata:
  annotations:
    oam.dev/dispatch-sig: <HMAC-SHA256(canonicalPayload, sharedSymmetricKey)>
```

The canonical payload is a deterministic serialisation of the fields the hub commits to:

```
componentName + namespace + definitionName + definitionRevision + propertiesHash + resourceVersion
```

The spoke recomputes the HMAC from the same fields using its copy of the shared symmetric key. If the values match, the Component is authentic and unmodified. If they don't, the spoke rejects the Component, emits a warning event, and does not proceed with rendering.

Since the shared symmetric key is **per-spoke**, a Component signed for `spoke-a` will always fail validation on `spoke-b` — cross-spoke replay is structurally prevented without any additional nonce or audience field.

The hub refreshes the signature on every reconcile that modifies the Component (properties change, definition upgrade, trait injection). The spoke treats a missing or invalid signature as a hard rejection — there is no fail-open mode for dispatch integrity.
