---
paths:
  - "**/*"
---

# Security
- .env Dateien NIEMALS committen
- Bei neuen Env-Variablen: .env.example aktualisieren (ohne Werte)
- Keine Secrets in Logs, Kommentaren oder Error Messages
- Input-Validierung bei allen externen Eingaben
