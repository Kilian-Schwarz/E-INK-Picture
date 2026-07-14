# PROGRESS

Manager-geführtes Log für den Weg zu v1.0. Nach jedem Task aktualisiert.
Gates: L1 statisch | L2 Render-Verifikation | L3 Hardware-in-the-Loop | L4 Panel-Foto | L5 Review.

## Aktueller Stand

Epic: Bootstrap | Task: Discovery (repo-cartographer) + Hardware-Baseline (hardware-validator) | Branch: chore/manager-bootstrap

## Test-Hardware

- Host: 10.33.0.121 (SSH-User/Key: wird durch hardware-validator ermittelt)
- Pi-Modell: unbestätigt (wird durch hardware-validator festgestellt)
- Panel: unbestätigt (epd7in3e oder epd7in5_V2)
- Kamera: unbestätigt → entscheidet, ob Gate L4 möglich ist

## Epics (Reihenfolge bewusst)

| Epic | Titel | Prio | Status |
|------|-------|------|--------|
| E1 | Display-Support & Render-Korrektheit (G1, G4) | P0 | offen |
| E2 | One-Command-Installation (G7, G10, G4) | P0 | offen |
| E3 | Designer: Canva-Niveau, Handy & PC (G2) | P0 | offen |
| E4 | Live-Datenquellen (G6, G9) | P1 | offen |
| E5 | Betrieb & Zuverlässigkeit (G3, G8) | P1 | offen |
| E6 | CI & Release (G5, G10) | P1 | offen |

## Erledigt

| Task | Beschreibung | Gates | Commit |
|------|--------------|-------|--------|
| Bootstrap | Subagent-Roster (.claude/agents/, 11 Agents) + PROGRESS.md angelegt | n/a (keine Code-Änderung) | — |

## Offen / Blockiert

- Discovery: repo-cartographer verifiziert Gap-Analyse G1–G10 — läuft
- Hardware-Baseline: hardware-validator ermittelt Pi-Modell, Panel, Kamera, SSH-Zugang — läuft
- CLAUDE.md-Korrektur (server/static/-Regel überholt): docs-writer — läuft
- E1.2 (eink_last_sent.png): wartet auf Spec — Vorbedingung für alle L3-Gates

## Übersprungene Gates

- (keine)

## Erkenntnisse

- (noch keine)
