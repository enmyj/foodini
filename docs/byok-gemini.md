# BYOK Gemini: User-Provided API Keys

## Goal

Let users bring their own Gemini API key so they pay for their own usage, without the server storing a shared key.

## Storage

Store the encrypted key in a hidden tab (`_config`) in the user's existing Google Sheet.

## Encryption: Envelope Encryption with Google Cloud KMS

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

## Why KMS over a local secret

- Key rotation handled by KMS — old ciphertexts still work
- KEK never touches disk or memory on our server
- Audit log of every decrypt via Cloud Audit Logs
- IAM controls on key access

## New GCP dependencies

- Service account with `roles/cloudkms.cryptoKeyEncrypterDecrypter`
- One KMS key ring + key (< $0.01/mo for expected volume)
- `cloud.google.com/go/kms` Go module

## New env vars

- `GCP_PROJECT` — project containing the KMS key
- `KMS_KEY_RESOURCE` — full resource name of the KMS key

## Migration

- `GEMINI_API_KEY` becomes optional / fallback
- Users without a stored key fall back to the server key (if set) or see a prompt to add theirs
- If the KMS key is rotated, old wrapped DEKs still decrypt (KMS handles this automatically)

## UI

- Settings panel where user can paste their Gemini API key
- Key is sent to the server once, encrypted, stored, never returned in plaintext
