# File Automation

A small full-stack playground that automatically sorts any file you upload into the right category folder (Photos, Documents, Audio, Video, Others) using an n8n workflow and a Python microservice. The goal is to explore how a UI, a Go API, a queue, and a low-code workflow engine can be wired together to build simple automations end-to-end.

## What to expect

1. Drop a file on the dashboard and hit **Upload & process**.
2. A 4-step progress indicator walks the job through **Upload → Queued → Processing → Done**.
3. When it finishes, a result card shows the detected **category** and where the file was saved (e.g. `Documents/invoice.pdf`).
4. On disk, the file is moved from `uploads/` into `uploads/<Category>/<original_name>`.

Under the hood those steps map to the backend statuses `PENDING → TRIGGERED → DONE` (or `FAILED`), pushed to the UI over WebSocket in real time.

## Flow

```
React UI  ──►  Go API  ──►  Redis queue  ──►  Go worker
                                                  │
                                                  ▼
                                          n8n workflow (webhook)
                                                  │
                                                  ▼
                                    Python service (/process)
                                                  │
                                                  ▼
                                   n8n callback ──►  Go /job/update
                                                  │
                                                  ▼
                                       WebSocket push to UI
```

- **Upload** — Frontend `POST /upload` sends the file to the Go backend.
- **Queue** — The backend stores the job in Redis and pushes it onto a `jobs` list.
- **Worker** — A Go goroutine pops jobs off the queue and fires the n8n webhook.
- **Process** — n8n calls the Python service, which classifies the file by extension and moves it into the right category folder.
- **Callback** — n8n posts the result back to `/job/update`, the backend updates Redis and broadcasts the change over WebSocket so the UI updates live.

## Tech stack

| Layer           | Tech                                    |
| --------------- | --------------------------------------- |
| Frontend        | React 18 + TypeScript + Vite            |
| API / worker    | Go (`net/http`, goroutines, WebSocket)  |
| Queue / store   | Redis 7                                 |
| Orchestration   | n8n (webhook → HTTP → callback)         |
| File logic      | Python 3 + Flask                        |
| Infra           | Docker Compose                          |

## Run it

```bash
cp .env.example .env
# edit .env: set N8N_ENCRYPTION_KEY (e.g. `openssl rand -base64 32`)
#           and pick a real N8N_BASIC_AUTH_PASSWORD
docker-compose up --build
```

- UI — http://localhost:5173
- API — http://localhost:8080
- n8n — http://localhost:5678 (basic auth — see `.env`)

Import an n8n workflow that hits `POST http://file-processor-service:5000/process` and then posts the response to `http://backend:8080/job/update`.

## How it scales

The design keeps each responsibility small and replaceable, which is where the "automation" side gets interesting:

- **Queue-based decoupling.** The API never processes files itself — it just persists the job and pushes to Redis. You can run multiple backend/worker replicas and Redis fairly distributes jobs via `BRPOP`.
- **Stateless services.** The Python service and the Go worker are stateless; scaling is just `docker compose up --scale file-processor-service=N`. All shared state lives in Redis and the `uploads/` volume.
- **n8n as the automation layer.** Adding new steps (virus scan, OCR, upload to S3, send a Slack message, tag in Notion…) is a drag-and-drop change in n8n — no redeploy of the Go or Python services needed.
- **Pluggable classification.** The category logic is a single `EXT_MAP` in `file-processor-service/app.py`. Swap it for a mime-type lookup, a rules engine, or an ML classifier and the rest of the pipeline is unchanged.
- **Live status via WebSocket.** The UI subscribes to job updates, so any new status a workflow emits (e.g. `SCANNING`, `UPLOADING`) shows up instantly without polling.

## Project layout

```
file-automation/
├── docker-compose.yml
├── frontend/                 # React + TypeScript UI
├── backend/                  # Go API + Redis worker + WebSocket
├── file-processor-service/   # Python Flask service (classify + move)
├── n8n-data/                 # n8n persisted workflows
└── uploads/                  # shared volume; sorted into category subfolders
```
