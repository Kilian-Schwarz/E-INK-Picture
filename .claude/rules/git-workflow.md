---
paths:
  - "**/*"
---

# Git Workflow
- Atomic commits nach JEDER verifizierten Änderung
- Conventional Commits: `type(scope): description`
- Pre-Commit: Tests + Linter + Type-Check müssen grün sein
- Keine .env oder Secrets committen — .gitignore prüfen
- Multi-Agent: eigener Feature Branch pro Agent, regelmäßig rebase
