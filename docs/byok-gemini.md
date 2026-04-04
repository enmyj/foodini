# BYOK Gemini: User-Provided API Keys

## Goal

Let users bring their own Gemini API key so they pay for their own usage, without the server storing a shared key.

## Storage

Store the encrypted key in a hidden tab (`_config`) in the user's existing Google Sheet.

## Option A: Envelope Encryption with Google Cloud KMS

1. Create a KEK (key encryption key) in Google Cloud KMS
2. **Encrypt flow** (user submits their API key):
   - Generate a random AES-256 DEK (data encryption key)
   - Encrypt the API key with the DEK (AES-GCM)
   - Call KMS to wrap (encrypt) the DEK
   - Store wrapped DEK + ciphertext in the `_config` sheet tab
3. **Decrypt flow** (user makes a Gemini request):
   - Read wrapped DEK + ciphertext from the sheet
   - Call KMS to unwrap (decrypt) the DEK
   - Decrypt the API key with the DEK
   - Create a `genai.Client` with the user's key for that request

### Why KMS

- Key rotation handled by KMS — old ciphertexts still work
- KEK never touches disk or memory on our server
- Audit log of every decrypt via Cloud Audit Logs
- IAM controls on key access

### New GCP dependencies

- Service account with `roles/cloudkms.cryptoKeyEncrypterDecrypter`
- One KMS key ring + key (< $0.01/mo for expected volume)
- `cloud.google.com/go/kms` Go module

### New env vars

- `GCP_PROJECT` — project containing the KMS key
- `KMS_KEY_RESOURCE` — full resource name of the KMS key

## Option B: XChaCha20-Poly1305 with local master key

No cloud KMS dependency. A 32-byte master key is loaded from an env var and used directly as a symmetric AEAD key. Simpler, zero external calls, good fit for a single-binary deploy.

### How it works

1. **Master key**: `ENCRYPTION_KEY` env var — 32 bytes, hex-encoded (64 hex chars). Generate with `openssl rand -hex 32`.
2. **Encrypt flow** (user submits their API key):
   - Generate a 24-byte random nonce (`crypto/rand`)
   - Seal the API key with `chacha20poly1305.NewX()` using the master key + nonce
   - Store `nonce || ciphertext` (base64-encoded) in the `_config` sheet tab
3. **Decrypt flow** (user makes a Gemini request):
   - Read the blob from the sheet, base64-decode
   - Split into nonce (first 24 bytes) and ciphertext (rest)
   - Open with the same AEAD + nonce
   - Create a `genai.Client` with the decrypted key

### Go sketch

```go
import "golang.org/x/crypto/chacha20poly1305"

// setup (once at startup)
key, _ := hex.DecodeString(os.Getenv("ENCRYPTION_KEY")) // 32 bytes
aead, _ := chacha20poly1305.NewX(key)                    // XChaCha20-Poly1305

// encrypt
nonce := make([]byte, aead.NonceSize()) // 24 bytes
rand.Read(nonce)
sealed := aead.Seal(nonce, nonce, plaintext, nil) // nonce || ciphertext+tag
blob := base64.StdEncoding.EncodeToString(sealed)

// decrypt
raw, _ := base64.StdEncoding.DecodeString(blob)
nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
plaintext, _ := aead.Open(nil, nonce, ciphertext, nil)
```

### Trade-offs vs KMS

| | KMS | Local key |
|---|---|---|
| External dependency | GCP KMS API calls on every decrypt | None |
| Key rotation | Automatic (KMS manages old versions) | Manual — re-encrypt all stored keys if rotated |
| Audit log | Built-in via Cloud Audit Logs | Roll your own |
| Latency | ~20-50ms per KMS call | Zero (in-process) |
| Cost | ~$0.01/mo | Free |
| Complexity | Service account + IAM setup | One env var |

For a single-server personal app, Option B is simpler and sufficient. KMS becomes worth it if multiple services need decryption access or you need key rotation without re-encryption.

### New env vars (Option B)

- `ENCRYPTION_KEY` — 64 hex chars (32 bytes). Generate: `openssl rand -hex 32`

## Migration

- `GEMINI_API_KEY` becomes optional / fallback
- Users without a stored key fall back to the server key (if set) or see a prompt to add theirs
- If the KMS key is rotated, old wrapped DEKs still decrypt (KMS handles this automatically)

## UI

- Settings panel where user can paste their Gemini API key
- Key is sent to the server once, encrypted, stored, never returned in plaintext
