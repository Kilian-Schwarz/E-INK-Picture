---
paths:
  - "docker-compose*.yml"
  - "Dockerfile*"
  - ".dockerignore"
---

# Docker
- Multi-stage Builds, explizite Image-Versionen (kein :latest)
- .dockerignore pflegen (node_modules, .env, .git)
- Keine Secrets in Dockerfile/docker-compose.yml
- Health Checks für Services, non-root User
