# Food Tracker

A personal food and macro tracking app built with Go and Svelte 5. Log meals in plain English using Google Gemini, store data in your own Google Sheets, and authenticate with Google OAuth2 — no third-party database, no app-level API costs.

- **Backend**: Go (single binary with embedded frontend)
- **Frontend**: Svelte 5 + Vite
- **Auth**: Sign in with Google (OAuth2)
- **Storage**: Google Sheets (your own Drive)
- **AI**: Google Gemini (your own quota, via your OAuth token)

---

## Prerequisites

- Go 1.22+
- Node.js 20+
- A Google account
- [mise](https://mise.jdx.dev) (optional but recommended for task running)

---

## Google Cloud Project Setup (one-time)

### 1. Create or select a project

Go to [https://console.cloud.google.com](https://console.cloud.google.com) and create a new project or select an existing one.

### 2. Enable APIs

Navigate to **APIs & Services → Library** and enable:

- **Google Sheets API**
- **Google Drive API**
- **Generative Language API**

### 3. Configure the OAuth consent screen

Go to **APIs & Services → OAuth consent screen**:

1. Set **User type** to **External**
2. Fill in **App name** and **Support email**
3. Add the following OAuth scopes:
   - `.../auth/spreadsheets`
   - `.../auth/drive.file`
   - `.../auth/generative-language`
4. Under **Test users**, add your own Google account

### 4. Create OAuth credentials

Go to **Credentials → Create Credentials → OAuth 2.0 Client ID**:

1. Set **Application type** to **Web application**
2. Under **Authorized redirect URIs**, add: `http://localhost:8080/auth/callback`
3. Click **Create** and note the **Client ID** and **Client Secret**

---

## Local Setup

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```env
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
COOKIE_SECRET=$(openssl rand -hex 32)   # must be 64+ hex chars
COOKIE_SECURE=false                      # set true in production (HTTPS)
PORT=8080
```

---

## Running (Development)

### With mise

```bash
mise run dev-backend   # terminal 1: Go server on :8080
mise run dev-frontend  # terminal 2: Vite dev server on :5173 (open this in browser)
```

### Without mise

```bash
# Terminal 1 — Go backend
export $(grep -v '^#' .env | xargs) && go run main.go

# Terminal 2 — Svelte frontend
cd frontend && npm run dev
```

Open [http://localhost:5173](http://localhost:5173) in your browser.

---

## Production Build

### With mise

```bash
mise run build   # builds frontend, then compiles Go binary
mise run run     # runs ./foodtracker
```

### Without mise

```bash
cd frontend && npm run build && cd ..
go build -o foodtracker .
export $(grep -v '^#' .env | xargs) && ./foodtracker
```

The resulting `foodtracker` binary embeds the built frontend and serves everything from a single process on the configured `PORT`.

---

## Deploying to Google Cloud Run

### First-time setup

Enable APIs and create the Artifact Registry repo (once per project):

```fish
set PROJECT_ID foodini-489420
gcloud config set project $PROJECT_ID
gcloud services enable run.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com

gcloud artifacts repositories create foodtracker \
  --repository-format=docker \
  --location=us-west1
```

Create secrets from your `.env` (once, or when values change):

```fish
set GOOGLE_CLIENT_ID (grep ^GOOGLE_CLIENT_ID .env | cut -d= -f2-)
set GOOGLE_CLIENT_SECRET (grep ^GOOGLE_CLIENT_SECRET .env | cut -d= -f2-)
set COOKIE_SECRET (grep ^COOKIE_SECRET .env | cut -d= -f2-)
set GEMINI_API_KEY (grep ^GEMINI_API_KEY .env | cut -d= -f2-)

echo -n $GOOGLE_CLIENT_ID | gcloud secrets create google-client-id --data-file=-
echo -n $GOOGLE_CLIENT_SECRET | gcloud secrets create google-client-secret --data-file=-
echo -n $COOKIE_SECRET | gcloud secrets create cookie-secret --data-file=-
echo -n $GEMINI_API_KEY | gcloud secrets create gemini-api-key --data-file=-

set PROJECT_NUMBER (gcloud projects describe $PROJECT_ID --format='value(projectNumber)')
for SECRET in google-client-id google-client-secret cookie-secret gemini-api-key
  gcloud secrets add-iam-policy-binding $SECRET \
    --member="serviceAccount:$PROJECT_NUMBER-compute@developer.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"
end
```

### Deploying changes

Build and push a new image, then redeploy:

```fish
set PROJECT_ID foodini-489420
gcloud auth print-access-token | sudo docker login -u oauth2accesstoken --password-stdin us-west1-docker.pkg.dev
sudo docker build -t us-west1-docker.pkg.dev/$PROJECT_ID/foodtracker/app:latest .
sudo docker push us-west1-docker.pkg.dev/$PROJECT_ID/foodtracker/app:latest

gcloud run deploy foodtracker \
  --image us-west1-docker.pkg.dev/$PROJECT_ID/foodtracker/app:latest \
  --region us-west1 \
  --platform managed \
  --allow-unauthenticated \
  --min-instances 0 \
  --max-instances 1 \
  --memory 256Mi \
  --set-env-vars "COOKIE_SECURE=true" \
  --set-secrets "GOOGLE_CLIENT_ID=google-client-id:latest,GOOGLE_CLIENT_SECRET=google-client-secret:latest,COOKIE_SECRET=cookie-secret:latest,GEMINI_API_KEY=gemini-api-key:latest"
```

### Updating a secret

```fish
echo -n "new-value" | gcloud secrets versions add SECRET-NAME --data-file=-
# then redeploy to pick up the new version
```

---

## Docker

### With mise

```bash
mise run docker-build   # builds the Docker image
mise run docker-run     # runs the container (reads .env)
```

### Without mise

```bash
# Build
docker build -t foodtracker .

# Run (requires .env in the current directory)
docker run --env-file .env -p 8080:8080 foodtracker
```

---

## Environment Variables

| Variable | Description | Example |
|---|---|---|
| `GOOGLE_CLIENT_ID` | OAuth 2.0 Client ID from GCP | `123456789.apps.googleusercontent.com` |
| `GOOGLE_CLIENT_SECRET` | OAuth 2.0 Client Secret from GCP | `GOCSPX-...` |
| `COOKIE_SECRET` | 64+ character secret used for session cookie encryption | `openssl rand -hex 32` |
| `COOKIE_SECURE` | Set `true` in production to restrict cookies to HTTPS | `false` |
| `PORT` | Port the server listens on | `8080` |

---

## How It Works

1. **Sign in with Google** — OAuth2 grants the app access to your Google Sheets, Drive, and Gemini quota. No credentials are stored on any server; your OAuth token lives in an encrypted session cookie.

2. **Automatic spreadsheet creation** — On first login, a "Food Tracker" spreadsheet is created in your Google Drive. All data is stored there; you can open and edit it directly at any time.

3. **Natural language logging** — Tap **+** and describe what you ate in plain English (e.g. "two scrambled eggs and a slice of sourdough toast"). Gemini estimates calories, protein, carbs, and fat, then logs the entry to your sheet.

4. **Today view** — Meals are grouped by type (breakfast, snack, lunch, dinner) with per-meal and daily macro totals displayed.

5. **Week view** — Toggle to a 7-day summary for a high-level overview of your intake.

6. **Inline editing** — Tap any field in the log to edit it directly. Changes are saved back to your Google Sheet.

7. **Activity notes** — Add a daily activity note at the bottom of the today view (e.g. "30 min run, 5k").
