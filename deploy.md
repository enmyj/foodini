# Deployment Guide

Deployed at `https://food.ianmyjer.com` via Cloudflare tunnel → Traefik → Kubernetes on the Pi.

Manifests live in `~/repos/pi/kube/manifests/` (foodini.yaml, ingress.yaml).

---

## First-time setup

### 1. Generate COOKIE_SECRET

Must be 64+ hex characters:

```bash
openssl rand -hex 32
```

### 2. Create the Kubernetes secret

```bash
kubectl create secret generic foodini-secret \
  --from-literal=google-client-secret=<google-client-secret> \
  --from-literal=cookie-secret=<output-from-step-1> \
  --from-literal=gemini-api-key=<gemini-api-key>
```

### 3. Verify Google OAuth redirect URI

In the Google Cloud Console, ensure `https://food.ianmyjer.com/auth/callback` is listed under authorized redirect URIs for the OAuth client.

---

## Building and deploying

### Build the image

`imagePullPolicy: Never` means the image must exist in k3s's local containerd store before deploying.

**Option A — build on dev machine, copy to Pi (cross-compile):**

For a 32-bit ARMv7 OS (Raspberry Pi OS 32-bit):
```bash
docker buildx build --platform linux/arm/v7 -t foodini:latest --output type=docker,dest=foodini.tar .
scp foodini.tar pi@<pi-ip>:~/
ssh pi@<pi-ip> "sudo k3s ctr images import ~/foodini.tar"
```

For a 64-bit ARM64 OS (Raspberry Pi OS 64-bit):
```bash
docker buildx build --platform linux/arm64 -t foodini:latest --output type=docker,dest=foodini.tar .
scp foodini.tar pi@<pi-ip>:~/
ssh pi@<pi-ip> "sudo k3s ctr images import ~/foodini.tar"
```

> The Dockerfile uses `--platform=$BUILDPLATFORM` for all build stages so the
> frontend (bun) and Go compiler always run natively — only the output binary
> targets the Pi's architecture.

**Option B — build directly on the Pi:**

```bash
docker build -t foodini:latest .
sudo docker save foodini:latest | sudo k3s ctr images import -
```

### Apply manifests

```bash
kubectl apply -f ~/repos/pi/kube/manifests/foodini.yaml
kubectl apply -f ~/repos/pi/kube/manifests/ingress.yaml
```

### Verify

```bash
kubectl get pods -l app=foodini
kubectl logs -l app=foodini --tail=20
```

---

## Updating

Rebuild the image (step above), then restart the pod to pick up the new image:

```bash
kubectl rollout restart deployment/foodini
```

---

## Direct binary deployment (ARMv7, no Docker)

Use this if you want to run the binary directly on a Pi without k3s/Docker.
The binary embeds the frontend, so only one file needs to be copied.

### Build

```bash
# Build frontend first (embeds into the Go binary)
mise run build-frontend

# Cross-compile for ARMv7 (Raspberry Pi 2/3 32-bit OS)
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 ~/go-sdk/go/bin/go build -o foodtracker-armv7 .

# Package it
tar czf foodtracker-armv7.tar.gz foodtracker-armv7
```

### Copy and run

```bash
scp foodtracker-armv7.tar.gz pi@<pi-ip>:~/

ssh pi@<pi-ip> "tar xzf foodtracker-armv7.tar.gz"
```

Then on the Pi, run it with your env vars:

```bash
GOOGLE_CLIENT_ID=... \
GOOGLE_CLIENT_SECRET=... \
REDIRECT_URL=https://food.ianmyjer.com/auth/callback \
COOKIE_SECRET=<64-hex-chars> \
COOKIE_SECURE=true \
GEMINI_API_KEY=... \
PORT=8080 \
./foodtracker-armv7
```

Or put the vars in a `.env` file and use:

```bash
export $(grep -v '^#' .env | xargs) && ./foodtracker-armv7
```

To run as a systemd service, create `/etc/systemd/system/foodtracker.service`:

```ini
[Unit]
Description=Food Tracker
After=network.target

[Service]
EnvironmentFile=/home/pi/.env
ExecStart=/home/pi/foodtracker-armv7
Restart=always
User=pi

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now foodtracker
```

---

## Env vars reference

| Var | Where set | Notes |
|-----|-----------|-------|
| `GOOGLE_CLIENT_ID` | foodini.yaml (plaintext) | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | `foodini-secret` k8s secret | OAuth client secret |
| `REDIRECT_URL` | foodini.yaml (plaintext) | `https://food.ianmyjer.com/auth/callback` |
| `COOKIE_SECRET` | `foodini-secret` k8s secret | 64+ hex chars, generated with `openssl rand -hex 32` |
| `COOKIE_SECURE` | foodini.yaml (plaintext) | `"true"` in prod |
| `PORT` | foodini.yaml (plaintext) | `8080` |
| `GEMINI_API_KEY` | `foodini-secret` k8s secret | Gemini API key |
