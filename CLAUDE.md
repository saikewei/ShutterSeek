# ShutterSeek Project Notes

## Auto-commit Rule
- After completing each small feature/fix, auto-commit with a descriptive message
- Do NOT push unless explicitly asked

## Project Stack
- Backend: Go/Gin + GORM + pgx + go-redis
- Frontend: Vue 3 + TypeScript + Vite + Tailwind CSS v4 + Headless UI
- Database: PostgreSQL with pgvector
- Deployment: Docker Compose on NAS via GitHub Actions + Tailscale

## Database
- Host: postgres-main, User: photo_user, DB: photo_search
- Credentials via SHUTTERSEEK_ env vars (never in config.yaml)

## Key Conventions
- Backend: handler → service → model layers
- Frontend: views/ for pages, components/ for reusable components, api/ for API clients
- Cursor-based pagination: taken_at DESC, id DESC
- Redis cache for first page only
