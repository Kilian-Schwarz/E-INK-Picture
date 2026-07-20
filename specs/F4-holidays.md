# F4: Feiertage-Widget (`widget_holidays`)

> **Folge-Task zum Pilot F7.** Dieses Spec folgt `docs/adding-a-widget.md` und
> spiegelt die Struktur von `specs/F7-year-progress.md`. Wo dieses Spec und die
> Doku sich widersprechen, gilt die Doku; wo Doku und
> `TestWidgetRegistrationCompleteness` sich widersprechen, gilt der Test.

> **Stand: geplant.** Nichts davon ist implementiert. Alle Datums- und
> Zählwerte in diesem Text sind von Hand nachgerechnet und müssen als
> Tabellentest gepinnt werden — nicht als Prosa stehen bleiben.

## Ziel

Ein Element vom Typ `widget_holidays` zeigt den nächsten gesetzlichen Feiertag
mit Countdown oder eine Liste der nächsten N Feiertage für ein wählbares
Bundesland — rein lokal berechnet, ohne Netzwerk — und wird von
Canvas-Preview und Panel-Renderer aus **derselben** `WidgetTextContent`-Dispatch
bedient.

---

## Architekturentscheidung: lokale Berechnung, keine API

**Empfehlung: lokale Berechnung. Klar, ohne Vorbehalt.**

Bewertet wurden `openholidaysapi.org` und `feiertage-api.de` (beide frei, ohne
Key) gegen eine Regeltabelle im Binary.

| Kriterium | Lokal | API |
|---|---|---|
| Offline (E5.5-Härtung, Pi ohne Netz nach Boot) | funktioniert immer | leer bis zum ersten erfolgreichen Fetch; danach nur so gut wie der persistente Cache |
| Golden-Harness | direkt möglich (Uhr-Seam reicht) | braucht zusätzlich einen injizierten **Datenseam**, sonst laut `docs/adding-a-widget.md` §5 gar nicht golden-fähig |
| Infrastruktur | keine | Positiv-Cache + TTL + `data/cache/`-Persistenz + `failCache`-Präfix + Body-Limit + Stale-Pfad (`weather.go`-Muster, ~150 Zeilen) |
| Provider-Risiko | keins | Abschaltung, Schema-Änderung, Rate-Limit, TLS-Ablauf |
| Korrektheit | abhängig von unserer Tabelle | abhängig von fremder Tabelle, die wir nicht testen können |
| Gesetzesänderung (z. B. Frauentag BE 2019, MV 2023) | Code-Änderung + Release nötig | zieht automatisch nach |
| Kosten | einmalig 16-Zeilen-Regeltabelle + Osterformel (~80 Zeilen) | dauerhaft ein Netzpfad mehr |

**Der einzige echte Punkt für die API** ist die letzte Zeile: ändert ein
Bundesland sein Feiertagsgesetz, muss bei lokaler Berechnung jemand die Tabelle
anfassen. Das ist real — die letzten drei Änderungen (Reformationstag als
Dauerfeiertag in HB/HH/NI/SH ab 2018, Frauentag BE ab 2019, Frauentag MV ab
2023) liegen keine zehn Jahre zurück.

Er wiegt trotzdem nicht auf, und zwar aus drei Gründen:

1. Solche Änderungen werden **Monate bis Jahre vorher** beschlossen und
   betreffen je ein Bundesland. Ein Release-Zyklus reicht.
2. Der Ausfallmodus ist verschieden schlimm. Lokal-veraltet heißt: ein Feiertag
   fehlt, bis jemand die Tabelle nachzieht. API-ausgefallen heißt: das Widget
   zeigt `Keine Daten` oder — schlimmer, über den Stale-Pfad — irgendwann
   einen Feiertag, der vorbei ist. Auf einem Wandpanel ohne Bedienelemente ist
   „still veraltet" der teurere Fehler.
3. Ein Provider löst das Problem nicht, er verschiebt es. Auch
   `openholidaysapi.org` pflegt eine Tabelle von Hand; wir tauschen unsere
   testbare Tabelle gegen eine ungetestete fremde plus einen Netzpfad.

Dazu kommt der Punkt, der die Entscheidung endgültig macht: **die Domäne ist
vollständig deterministisch.** Es gibt keinen Messwert, keine Prognose, keine
Quelle der Wahrheit außerhalb eines Gesetzestextes. Ein Netzaufruf für eine
Funktion, die reine Arithmetik über `(Jahr, Bundesland)` ist, ist per
Konstruktion die schlechtere Lösung — er fügt Latenz, Ausfall und
Nicht-Determinismus zu etwas hinzu, das keines davon braucht.

**Absicherung gegen die Veraltungs-Schwäche** (Teil dieses Tasks): über der
Regeltabelle steht ein Kommentar mit Rechtsstand und Datum, und
`TestHolidayRuleTableStamp` prüft, dass eine Konstante
`holidayRulesAsOf = "2026-01-01"` existiert und in der Tabellendokumentation
genannt ist. Das macht die Tabelle nicht aktuell, aber es macht sichtbar, wie
alt sie ist.

---

## Kontext

### Zentrale Dispatch (unverändert lassen, nur erweitern)

`server/internal/services/preview.go` — `WidgetTextContent` (Signatur `:389`,
`switch` `:390-416`). Neuer Fall:

```go
case "widget_holidays":
    return s.fillHolidaysContent(props), true
```

Konsumenten unverändert: `drawElement` (`preview.go:418ff`) und
`handlers/widgets.go:43` (`POST /api/widget_content`).

### Neue Datei

`server/internal/services/widget_holidays.go` mit
`func (s *PreviewService) fillHolidaysContent(props map[string]any) string`.

### Wiederverwendung statt Neubau

- Uhr: `s.nowOrDefault()` (`preview.go:92-108`) — der von F7 eingeführte Seam.
- Deutsche Wochentage: `germanWeekdaysShort` (`services/locale.go:19`,
  `{"So","Mo","Di","Mi","Do","Fr","Sa"}`). **Nicht** neu anlegen.
- Clamping: `clampInt` (`widget_progress.go:196`).
- Property-Zugriff: `GetPropString` / `GetPropInt` (`design.go:977` / `:1005`).

### Kein globales Standort-Setting — bewusst

**Verifiziert:** die Codebasis hat heute **keine** globale Standort-Einstellung.
`settings.go` kennt Display, Refresh-Intervall, Render-Qualität, Dithering,
Kalibrierung und Sleep-Window, aber keinen Ort; Geokoordinaten existieren nur
als `lat`/`lon`-Properties **pro Element** am Wetter-Widget.

Das Bundesland ist deshalb in F4 eine **Widget-Property** (`state`), analog zu
`lat`/`lon` beim Wetter und `timezone` bei Clock/Progress.

**Querschnittsthema für F6/F5, ausdrücklich nicht hier:** ein globales
`location`-Setting (Bundesland + Koordinaten + Timezone an einer Stelle) würde
heute schon drei Widgets bedienen — Wetter, Forecast, Feiertage — und mit F4
kommt das dritte dazu. Wer F6/F5 plant, sollte das aufgreifen; F4 baut es
**nicht**, sondern liefert nur einen weiteren Beleg dafür, dass es fehlt. Wenn
es kommt, wird `state` zu einem Override über dem globalen Wert, die Property
bleibt.

### Default-Bundesland: `DE` (bundesweit), nicht ein konkretes Land

Ein frisch aus der Palette gezogenes Widget muss einen Default haben. Jedes
konkrete Bundesland wäre für 15 von 16 Nutzern falsch. Deshalb gibt es den
Sentinel-Wert `DE` = **nur die neun bundesweiten Feiertage**, und der ist
Default.

Begründung über die Fehlerrichtung: `DE` ist eine echte Teilmenge jedes
Bundeslandes. Der Fehler bei falscher Konfiguration ist damit immer eine
**Auslassung** („Fronleichnam fehlt in der Liste"), nie eine **Falschaussage**
(„Morgen ist frei" an einem Arbeitstag). Auf einem Panel ohne Interaktion ist
die Falschaussage der teurere Fehler — sie führt zu einer falschen Handlung,
die Auslassung nur zu einer unvollständigen Anzeige.

---

## Property-Schema

Minimal, sieben Felder, alle optional:

| Property | Typ | Default | Wertebereich |
|---|---|---|---|
| `state` | string | `"DE"` | `DE` \| `BW` \| `BY` \| `BE` \| `BB` \| `HB` \| `HH` \| `HE` \| `MV` \| `NI` \| `NW` \| `RP` \| `SL` \| `SN` \| `ST` \| `SH` \| `TH` |
| `layout` | string | `"next_countdown"` | `next` \| `next_countdown` \| `list` \| `custom` |
| `count` | number | `3` | 1..10 (nur bei `layout=list`) |
| `timezone` | string | `""` | IANA-Name; `""` = Serverzeit |
| `customTemplate` | string | `"%name% (%date%)"` | nur bei `layout=custom` |
| `fontSize` | number | `13` | 8..200 |
| `color` | string | `"#000000"` | Palettenfarbe |
| `textAlign` | string | `"left"` | left \| center \| right |

Unbekannter `state` → Fallback `DE`. Unbekannter `layout` → Fallback
`next_countdown`. `count` außerhalb → geklemmt.

Kein `label`, keine `showWeekday`-Flags, keine Farbcodierung, kein
Datumsformat-Feld.

---

## Domänenmodell

### Osterformel — Anonyme Gregorianische Berechnung (Meeus/Jones/Butcher)

Ganzzahlige Division, ganzzahliger Modulo, alles `int`. Für ein Jahr `Y`:

```
a = Y mod 19
b = Y div 100
c = Y mod 100
d = b div 4
e = b mod 4
f = (b + 8) div 25
g = (b - f + 1) div 3
h = (19a + b - d - g + 15) mod 30
i = c div 4
k = c mod 4
l = (32 + 2e + 2i - h - k) mod 7
m = (a + 11h + 22l) div 451
n = h + l - 7m + 114
Monat = n div 31          (3 = März, 4 = April)
Tag   = (n mod 31) + 1
```

Ergebnis ist **Ostersonntag** im Gregorianischen Kalender, als
`time.Date(Y, Monat, Tag, 0,0,0,0, loc)`.

Pflicht-Stützstellen (Tabellentest `TestEasterSunday`):

| Jahr | Ostersonntag |
|---|---|
| 2024 | 31.03. |
| 2025 | 20.04. |
| 2026 | 05.04. |
| 2027 | 28.03. |
| 2028 | 16.04. |
| 2030 | 21.04. |
| 2038 | 25.04. (spätestmöglicher Termin) |

### Osterabhängige Feiertage (Offset in Tagen zum Ostersonntag)

| Feiertag | Offset | Gültig in |
|---|---|---|
| Karfreitag | −2 | alle |
| Ostermontag | +1 | alle |
| Christi Himmelfahrt | +39 | alle |
| Pfingstmontag | +50 | alle |
| Fronleichnam | +60 | BW, BY, HE, NW, RP, SL |

Offsets werden über `AddDate(0, 0, offset)` auf das Ostersonntag-Datum
angewendet — **nie** über `time.Duration`-Addition, sonst kippt der Termin an
DST-Tagen (Ostern und Himmelfahrt liegen beide in der Nähe der
Märzumstellung bzw. dahinter).

### Feste Feiertage

| Datum | Feiertag | Gültig in |
|---|---|---|
| 01.01. | Neujahr | alle |
| 06.01. | Heilige Drei Könige | BW, BY, ST |
| 08.03. | Internationaler Frauentag | BE, MV |
| 01.05. | Tag der Arbeit | alle |
| 15.08. | Mariä Himmelfahrt | SL |
| 20.09. | Weltkindertag | TH |
| 03.10. | Tag der Deutschen Einheit | alle |
| 31.10. | Reformationstag | BB, HB, HH, MV, NI, SH, SN, ST, TH |
| 01.11. | Allerheiligen | BW, BY, NW, RP, SL |
| 25.12. | 1. Weihnachtstag | alle |
| 26.12. | 2. Weihnachtstag | alle |

### Buß- und Bettag — beweglich, nur Sachsen

Nur `SN`. Regel: **der Mittwoch vor dem 23. November**, d. h. äquivalent und
einfacher zu implementieren: *derjenige Mittwoch, der in das Intervall
16.11.–22.11. fällt*. Das „vor" ist **strikt** — fällt der 23.11. selbst auf
einen Mittwoch, ist der Feiertag der 16.11.

Stützstellen (Tabellentest, inklusive der strikten Kante):

| Jahr | 23.11. ist ein | Buß- und Bettag |
|---|---|---|
| 2022 | Mittwoch | **16.11.2022** (strikte Kante) |
| 2025 | Sonntag | 19.11.2025 |
| 2026 | Montag | 18.11.2026 |
| 2027 | Dienstag | 17.11.2027 |
| 2028 | Donnerstag | 22.11.2028 |

### Ausdrücklich nicht enthalten

- **Augsburger Friedensfest (08.08.)** — kommunaler Feiertag, gilt nur im
  Stadtgebiet Augsburg, nicht in Bayern. **Out of Scope**, weil das Widget
  keine Gemeindeauflösung hat und nie bekommen soll.
- **Mariä Himmelfahrt in Bayern** — gilt nur in Gemeinden mit überwiegend
  katholischer Bevölkerung (rund 1700 von 2056). Dieselbe Begründung: keine
  Gemeindeauflösung. In `SL` gilt der Tag landesweit und ist deshalb drin.
- **Fronleichnam in Sachsen und Thüringen** — ebenfalls nur in einzelnen
  Gemeinden. Nicht enthalten.
- **Ostersonntag und Pfingstsonntag** — bewusst in **keinem** Bundesland
  gelistet. Zwei Gründe: die rechtliche Behandlung ist zwischen den Ländern
  uneinheitlich und in der Literatur strittig (BB/HE), und beide fallen
  ohnehin immer auf einen Sonntag, sind also für einen „nächster freier Tag"-
  Countdown reines Rauschen. Diese Auslassung ist eine Entscheidung, kein
  Versehen, und gehört als Kommentar in die Tabelle.

### Feiertagsanzahl je Bundesland (Testtabelle, verbindlich)

Bundesweit gemeinsam sind 9: Neujahr, Karfreitag, Ostermontag, Tag der Arbeit,
Christi Himmelfahrt, Pfingstmontag, Tag der Deutschen Einheit, 1. und
2. Weihnachtstag.

| Code | Land | Anzahl | Zusätzlich zu den 9 |
|---|---|---|---|
| `DE` | (bundesweit) | **9** | — |
| `BE` | Berlin | 10 | Frauentag |
| `BB` | Brandenburg | 10 | Reformationstag |
| `HB` | Bremen | 10 | Reformationstag |
| `HH` | Hamburg | 10 | Reformationstag |
| `HE` | Hessen | 10 | Fronleichnam |
| `NI` | Niedersachsen | 10 | Reformationstag |
| `SH` | Schleswig-Holstein | 10 | Reformationstag |
| `MV` | Mecklenburg-Vorpommern | 11 | Frauentag, Reformationstag |
| `NW` | Nordrhein-Westfalen | 11 | Fronleichnam, Allerheiligen |
| `RP` | Rheinland-Pfalz | 11 | Fronleichnam, Allerheiligen |
| `SN` | Sachsen | 11 | Reformationstag, Buß- und Bettag |
| `ST` | Sachsen-Anhalt | 11 | Heilige Drei Könige, Reformationstag |
| `TH` | Thüringen | 11 | Reformationstag, Weltkindertag |
| `BW` | Baden-Württemberg | **12** | Heilige Drei Könige, Fronleichnam, Allerheiligen |
| `BY` | Bayern | **12** | Heilige Drei Könige, Fronleichnam, Allerheiligen |
| `SL` | Saarland | **12** | Fronleichnam, Mariä Himmelfahrt, Allerheiligen |

Minimum 9 (`DE`) bzw. 10 unter den echten Ländern, Maximum 12.

### Kalendertage zählen — DST-fest

Der Countdown ist eine **Kalendertagsdifferenz**, keine Dauer. Eine Rechnung
über `holiday.Sub(today) / (24*time.Hour)` ist an den zwei DST-Tagen im Jahr um
einen Tag falsch.

Verbindliche Regel: beide Daten werden auf ihre **Zivildatums-Tripel**
`(y, m, d)` reduziert und als `time.Date(y, m, d, 0,0,0,0, time.UTC)`
neu gebaut; die Differenz dieser beiden UTC-Mitternachte, geteilt durch
`24*time.Hour`, ist die Tagesanzahl. UTC hat per Definition keine
DST-Sprünge, deshalb ist der Quotient exakt.

```
daysBetween(2028-02-28, 2028-04-14) == 46   // Schaltjahr, 29.02. gezählt
daysBetween(2027-02-28, 2027-04-14) == 45   // kein Schaltjahr
```

### Kandidatenfenster und Auswahl

1. `serverNow := s.nowOrDefault()` — **einmal**, danach `now := serverNow.In(loc)`.
2. `today = time.Date(now.Year(), now.Month(), now.Day(), 0,0,0,0, loc)`.
3. Kandidaten = Feiertage der Jahre `Y`, `Y+1`, `Y+2` (`Y = now.Year()`).
   Drei Jahre, nicht zwei: am 31.12. mit `count = 10` reichen zwei Jahre
   knapp nicht garantiert, drei immer (≥ 18 Einträge nach `today`).
4. Filter `datum >= today` — ein Feiertag, der **heute** ist, bleibt drin und
   zählt als `0` Tage.
5. Sortierung aufsteigend nach Datum; bei Gleichstand (kommt in der Tabelle
   nicht vor, aber die Sortierung muss total sein) nach Name.
6. `next` = erstes Element, `list` = erste `count` Elemente.

Die Liste ist **nie leer** — Neujahr des Folgejahres liegt immer im Fenster.

---

## Ausgabeformat

Referenzzeitpunkt aller Beispiele: **2026-07-20 12:00 Europe/Berlin**
(`goldenNow`, `golden_test.go:50`).

Nachrechnung (verbindlich, weil die goldene Datei daran hängt): 2026-07-20 ist
ein Montag. Nächster bundesweiter Feiertag ist der 03.10.2026; Tagesdifferenz
= 11 (Juli) + 31 (August) + 30 (September) + 3 (Oktober) = **75**. Der
03.10.2026 ist 75 Tage nach einem Montag, `75 mod 7 = 5` → **Samstag**.
31.10.2026 liegt 28 Tage später → ebenfalls **Samstag**. 18.11.2026 ist ein
**Mittwoch** (siehe Buß-und-Bettag-Tabelle).

Datumsformat: `%s, %02d.%02d.%04d` mit `germanWeekdaysShort` →
`Sa, 03.10.2026`.

| ID | Name | Beispiel (`state=DE`) |
|---|---|---|
| `next` | Nächster Feiertag | `Tag der Deutschen Einheit\nSa, 03.10.2026` |
| `next_countdown` | Nächster + Countdown | `Tag der Deutschen Einheit\nSa, 03.10.2026 (in 75 Tagen)` |
| `list` | Liste | siehe unten |
| `custom` | Custom Template | frei |

`list` mit `state=SN`, `count=3`:

```
Sa, 03.10.2026  Tag der Deutschen Einheit
Sa, 31.10.2026  Reformationstag
Mi, 18.11.2026  Buß- und Bettag
```

Datum zuerst, dann **zwei** Leerzeichen, dann der Name. Zeilen mit `\n`
getrennt, **kein** abschließendes `\n`.

### Countdown-Wortlaut (verbindlich)

| Tage `N` | Ausgabe |
|---|---|
| 0 | `heute` |
| 1 | `morgen` |
| ≥ 2 | `in N Tagen` |

`in 1 Tag` tritt damit nie auf — der Singularfall ist durch `morgen` abgedeckt.
Im `next_countdown`-Layout steht der Ausdruck in Klammern hinter dem Datum:
`Sa, 03.10.2026 (heute)` wäre widersprüchlich, deshalb gilt: bei `N == 0` und
`N == 1` lautet die zweite Zeile `Sa, 03.10.2026 (heute)` bzw. `(morgen)` —
das ist gewollt und wird als Beispiel gepinnt.

### Placeholders (`allPlaceholders`)

`%name%`, `%date%`, `%weekday%`, `%days%`, `%state%`

- `%date%` = `03.10.2026` (ohne Wochentag), `%weekday%` = `Sa`
- `%days%` = die **rohe Zahl** `75` (nicht `in 75 Tagen`) — damit ein Nutzer
  `noch %days% Tage` schreiben kann
- `%state%` = ausgeschriebener Landesname, `Deutschland` bei `DE`

Alle fünf müssen tatsächlich substituiert werden, sonst schlägt
`TestDeadPlaceholderRegistry` (`widget_registration_test.go:279`) fehl.

---

## Registrierungspunkte — alle acht

| # | Datei | Anker (heute) | Eintrag |
|---|---|---|---|
| 0 | `services/preview.go` | `WidgetTextContent` `:390-416` | `case "widget_holidays": return s.fillHolidaysContent(props), true` |
| 7 | `services/preview.go` | `widgetDefaultFontSizes` `:357-369` | `"widget_holidays": 13` |
| 8 | `services/widgets/layouts.go` | `allLayouts` `:19`, `allPlaceholders` `:86` | 4 Layouts + 5 Placeholders (oben) |
| 1 | `static/js/element-factory.js` | `defaultSizes` `:117-131` | `widget_holidays: { w: 380, h: 110 }` |
| 2 | `static/js/element-factory.js` | `getDefaultProperties` `:198-302` | Defaults spiegelbildlich zum Property-Schema |
| 3 | `static/js/properties-panel.js` | `getWidgetPropertyDefs` `:1079-…` | `state` als `select` mit `{value,label}`-Paaren (Muster: `widget_progress.period`, `:1142-1149`), `layout` als `select`, `count` als `number` min 1 max 10 |
| 4 | `static/js/widgets.js` | `getPreviewContent` Passthrough `:83` | `case 'widget_holidays':` |
| 5a | `static/js/widgets.js` | `getDefaultLayout` `:97-106` | `widget_holidays: 'next_countdown',` |
| 5b | `static/js/widgets.js` | `getPreviewFontSize` `:214-235` | `widget_holidays: 13,` — identisch zu Punkt 7 |
| 6 | `templates/designer.html` | Palette `:80-132` | `<div class="widget-item" data-type="widget_holidays">` mit Icon `DE` und Label `Feiertage` |

**Warum `w: 380`, `h: 110`, `fontSize: 13`** (der nicht offensichtliche Teil):
das Canvas-Label ist eine umbruchfähige `fabric.Textbox`, und `updatePreview`
setzt `label.set('width', w - 16)` (`widgets.js:263`). Die längste
Default-Zeile ist `Sa, 03.10.2026  Tag der Deutschen Einheit` = **41 Zeichen**.
Bei `fontSize: 13` und der Proportionalschrift sind das grob 41 × 6,9 ≈ 283 px;
mit `w = 380` bleiben `380 − 16 = 364` px, also ~80 px Reserve. Bei `w = 300`
(−16 = 284) wäre die Reserve ~1 px und jede Fontmetrik-Abweichung würde die
Zeile umbrechen. `fontSize: 13` statt 18 folgt außerdem den anderen
Listen-Widgets (`widget_forecast`/`calendar`/`news`, alle 13). `h = 110` trägt
`count = 3` bei 13 px Zeilenhöhe plus Padding; bei `count = 10` muss der Nutzer
das Element selbst vergrößern — das ist akzeptiert und kein Fehler.

---

## Akzeptanzkriterien

**AC1 — Dispatch bleibt Single Source.**
`WidgetTextContent("widget_holidays", props)` liefert `ok == true`. In
`widgets.js` kommt der Typ (Kommentare entfernt) in **genau drei** Rollen vor —
Passthrough-Case, `getDefaultLayout`, `getPreviewFontSize` — und in keiner
weiteren. `strings.Count(src, "widget_holidays") == 3`.

**AC2 — Kein JS-seitiges Datumsrechnen.**
`widgets.js` enthält **keine** der Zeichenketten `buildHolidaysContent`,
`holidaysContent`, `buildHolidays`, `easterSunday`, `getEaster`, `Feiertag`,
`bundesland`, `bussUndBettag`. Der Typ steht im Passthrough-Zweig, der
weiterhin `liveData.content` verbatim zurückgibt. Test:
`TestHolidaysCanvasPanelParity`, nach dem Muster von
`TestProgressCanvasPanelParity` (`widget_registration_test.go:350`).

**AC3 — Canvas == Panel, byteidentisch.**
Für eine feste Property-Menge liefert `POST /api/widget_content` exakt denselben
String wie der Direktaufruf `svc.fillHolidaysContent(props)`. Test vergleicht
byteweise. `{"type":"widget_holidays"}` **ohne** `properties` → HTTP 200 mit
nichtleerem `content`, nicht 400.

**AC4 — Alle acht Registrierungspunkte belegt.**
`TestWidgetRegistrationCompleteness` (`widget_registration_test.go:190`) läuft
für `widget_holidays` **grün und nicht als SKIP**. Der Test entdeckt das Widget
automatisch, sobald der `case` in `WidgetTextContent` steht — es ist keine Liste
zu pflegen. Punkt 5a/8 gelten, weil `fillHolidaysContent` sowohl
`props["layout"]` als auch `props["customTemplate"]` liest; beide Richtungen
werden geprüft.

**AC5 — Osterformel.**
`easterSunday(Y)` liefert für alle sieben Stützstellen der Tabelle oben das
exakte Datum. Zusätzlich: für jedes `Y` in `[1900, 2100]` liegt das Ergebnis im
Intervall `[22.03., 25.04.]` und ist ein **Sonntag** (`Weekday() == time.Sunday`)
— das ist ein Invariantentest über 201 Jahre und fängt jeden Vorzeichenfehler
in der Formel, den die Stützstellen durchlassen.

**AC6 — Feiertagsanzahl je Bundesland.**
Für `Y = 2026` liefert `holidaysForYear(Y, state)` für jeden der 17 Codes exakt
die in der Tabelle genannte Anzahl. Insbesondere:
- `DE` → 9 (Minimum)
- `HH` → 10, `BE` → 10 (wenigste unter den echten Ländern)
- `BW` → 12, `BY` → 12, `SL` → 12 (meiste)
- Jeder `DE`-Feiertag ist in **jedem** der 16 Länder enthalten (Teilmengen-
  Invariante — sie trägt die Begründung des `DE`-Defaults).
- Unbekannter Code `"XX"` → identisch zu `DE`, kein Fehler, kein Panic.

**AC7 — Per-Bundesland-Regeln, die erfahrungsgemäß schiefgehen.**
Für `Y = 2026` (Ostersonntag 05.04.2026):
- Fronleichnam = **04.06.2026** (Ostern +60), enthalten in genau
  `{BW, BY, HE, NW, RP, SL}` und in **keinem** anderen Land.
- Allerheiligen 01.11. in genau `{BW, BY, NW, RP, SL}`.
- Reformationstag 31.10. in genau
  `{BB, HB, HH, MV, NI, SH, SN, ST, TH}` — neun Länder, **nicht** in
  `{BW, BY, BE, HE, NW, RP, SL}`.
- Buß- und Bettag in genau `{SN}` und in keinem anderen Land.
- Mariä Himmelfahrt in genau `{SL}`, **nicht** in `BY`.
- Weltkindertag in genau `{TH}`, Frauentag in genau `{BE, MV}`.
- Der 08.08. (Augsburger Friedensfest) taucht in **keinem** Land auf — Test
  über alle 17 Codes und alle Jahre 2024–2030.

**AC8 — Buß- und Bettag, bewegliches Datum.**
Alle fünf Stützstellen der Tabelle oben, inklusive der strikten Kante 2022
(23.11. ist selbst ein Mittwoch → 16.11.). Zusätzliche Invariante über
`[2000, 2100]`: das Ergebnis ist immer ein Mittwoch und liegt immer im
Intervall `[16.11., 22.11.]`.

**AC9 — Jahresübergang (Off-by-one-Falle).**
`state = "DE"`, `timezone = "Europe/Berlin"`, `layout = "next_countdown"`:
- `2026-12-27 09:00` → nächster Feiertag ist **Neujahr, Fr, 01.01.2027**,
  Countdown **`in 5 Tagen`**. Nicht Neujahr 2026, nicht der 26.12.2026.
- `2026-12-25 09:00` → **1. Weihnachtstag, Fr, 25.12.2026**, Countdown
  **`heute`** (Feiertag von heute fällt nicht heraus).
- `2026-12-25 09:00`, `layout = "list"`, `count = 3` → drei Zeilen:
  25.12.2026, 26.12.2026, 01.01.2027 — die Liste läuft über die Jahresgrenze.
- `2026-12-31 23:00`, `count = 10` → genau 10 Zeilen, keine leere Zeile, keine
  Wiederholung (belegt das Drei-Jahres-Fenster).

**AC10 — Schaltjahr.**
- `2028-02-28`, `state = "DE"`, `layout = "next_countdown"` → Karfreitag
  **Fr, 14.04.2028** (Ostern 16.04.2028 − 2), Countdown **`in 46 Tagen`**.
  Der 29.02.2028 ist mitgezählt.
- Kontrolle ohne Schalttag: `daysBetween(2027-02-28, 2027-04-14) == 45`.
- `2028-02-29 08:00` ist ein gültiger Eingabezeitpunkt und liefert Content ohne
  Panic.

**AC11 — Countdown-Wortlaut und Kalendertagslogik.**
- `N == 0` → `heute`, `N == 1` → `morgen`, `N == 2` → `in 2 Tagen`.
  Die Zeichenkette `in 1 Tag` kommt in keiner Ausgabe vor.
- `23:59` am Vortag eines Feiertags → `morgen`, `00:01` am Feiertag → `heute`.
  Der Übergang hängt an der lokalen Mitternacht, nicht an 24-Stunden-Blöcken.
- DST-Kontrolle: der Countdown über den 29.03.2026 (Spring Forward) und den
  25.10.2026 (Fall Back) hinweg ist um **keinen** Tag verschoben —
  `daysBetween` rechnet in UTC-Mitternachten.

**AC12 — Timezone-Verhalten definiert.**
`timezone: "UTC"` und `timezone: "Europe/Berlin"` liefern am
`2026-12-31 23:30 UTC` unterschiedliche Ergebnisse: mit `Europe/Berlin` ist
lokal bereits der 01.01.2027 → Countdown `heute` für Neujahr; mit `UTC` noch
der 31.12.2026 → `morgen`. `timezone: "Mars/Olympus"` (ungültig) fällt **still**
auf Serverzeit zurück, wirft keinen Fehler. Zonenabhängige Tests laden über
einen `mustLoadLocation`-Helper mit `t.Skip` bei fehlender tzdata
(`widget_progress_test.go:20-27`).

**AC13 — Clock-once.**
`fillHolidaysContent` ruft `s.nowOrDefault()` **genau einmal** auf und leitet
Location, `today` und alle Kandidatenjahre daraus ab. Mechanisch geprüft: ein
AST-Test zählt die `nowOrDefault`-Aufrufe im Funktionskörper und schlägt bei
`!= 1` fehl. Direkte `time.Now()`-Aufrufe in `widget_holidays.go` sind
verboten (gleicher Test).

**AC14 — Defaults, ungültige Eingaben, kein Panic.**
- `props == nil` oder `{}` → gültige Ausgabe mit `state=DE`,
  `layout=next_countdown`; **nie** leerer String, **nie** Panic.
- `state: "Bayern"` / `"xx"` / `""` → Fallback `DE`.
- `layout: "wolkig"` → Fallback `next_countdown`.
- `count: 0` / `-5` / `9999` → geklemmt auf `[1, 10]`.
- `layout: "custom"` mit leerem `customTemplate` → Default-Template
  `"%name% (%date%)"`.
- `PreviewService{}`-Zerowert (`now == nil`) paniked nicht — der Zugriff läuft
  über `nowOrDefault()`.

**AC15 — `float64` in allen Tests.**
Jede numerische Property in jeder Test-Property-Map ist `float64(...)`:
`{"count": float64(3)}`, **nie** `{"count": 3}`. `GetPropInt`
(`design.go:1005-1017`) dekodiert nur `float64` und `string`; ein blankes `int`
fällt still in den Default. Ein Test mit `3` wäre grün, weil der Default
zufällig auch 3 ist, und würde damit nichts beweisen — genau dieser Bug steckte
in den F7-Tests. Mechanische Absicherung: ein AST-Test über
`widget_holidays_test.go` sucht in allen `map[string]any`-Literalen nach
`BasicLit` mit `token.INT` und schlägt fehl.

**AC16 — Zeichensatz.**
Jede Rune der Ausgabe ist `< U+0100`, also ASCII oder Latin-1. Damit sind
`ä`, `ö`, `ü`, `ß` erlaubt (`Buß- und Bettag`, `Mariä Himmelfahrt`,
`Christi Himmelfahrt`) — Präzedenzfall `germanMonths` mit `März`
(`locale.go:22`) —, aber **verboten** sind Unicode-Block- und
Rahmenzeichen, Gedankenstrich `–`, Geviertstrich `—`, typografische
Anführungszeichen und `…`. Die eingebettete Fallback-Schrift `goregular` hat
diese Glyphen nicht bzw. nicht zuverlässig; das Panel rendert dann Leerraum
oder Tofu, während der Browser-Canvas korrekt aussieht. Test:
`TestHolidaysCharsetSafe` über alle vier Layouts × alle 17 Codes.

**AC17 — Golden-Design, beide Displays.**
- `server/internal/services/testdata/designs/holidays.json` mit **drei**
  `widget_holidays`-Elementen: `(state=DE, layout=next_countdown)`,
  `(state=SN, layout=list, count=3)`, `(state=BY, layout=next)`.
  Jedes Element pinnt `fontFamily: "testfont.ttf"` (Pflicht, siehe
  `setupGoldenServicesVariant`, `golden_test.go:83-88`) und
  `timezone: "Europe/Berlin"` explizit.
- `"holidays"` steht in `goldenDesigns` (`golden_test.go:34`).
- `"holidays"` steht in `goldenTZDesigns` (`golden_test.go:56`) mit
  `"Europe/Berlin"` — sonst meldet ein Host ohne tzdata einen irreführenden
  Pixel-Diff statt zu skippen.
- `"holidays"` steht in `ditherDesigns` (`golden_test.go:344`). **Ohne diesen
  Eintrag läuft die Palettenprüfung für das Design überhaupt nicht** — das ist
  der leiseste der vier Einträge und der, der am ehesten vergessen wird.
- Golden-PNGs für **beide** Displays im **selben** Commit wie der
  Renderer-Code: `holidays__waveshare_7in3_e.png` und
  `holidays__waveshare_7in5_v2.png`.
- Die elf bestehenden Golden-Dateien bleiben **byteidentisch**.

**AC18 — Palettentreue auf beiden Displays.**
`assertPaletteExactness` (`golden_test.go:261`) läuft für `holidays` über
`goldenDisplays` (beide Profile) und alle drei `RenderQuality`-Stufen grün:
- `DisplayWaveshare75V2` (S/W): PNG-Palette exakt gleich `cfg.Colors` in Länge
  und Reihenfolge, keine Fremdfarbe, ≥ 2 Palettenfarben genutzt.
- `DisplayWaveshare73E` (6 Farben): dieselben drei Aussagen.

**AC19 — Monochrom-Lesbarkeit.**
Das Widget kodiert **keine** Information über Farbe — es gibt keine
Feiertagstypen-Einfärbung, keinen roten „heute"-Marker. Jede Aussage steht als
Text. Prüfung: `holidays__waveshare_7in5_v2.png` allein öffnen und lesen; alle
drei Elemente sind vollständig verständlich.

**AC20 — Kein Netzwerk, keine Dependency.**
`go.mod` und `go.sum` bleiben unverändert. `widget_holidays.go` enthält keinen
`net/http`-Import und keinen Fetch. Alle Holiday-Tests laufen auf einem
`&PreviewService{}` ohne verdrahtete Abhängigkeit. Der Offline-Render-Test
(`offline_render_test.go`) braucht **keine** Anpassung — wenn er eine braucht,
ist versehentlich ein Netzpfad entstanden.

**AC21 — Font-Size-Parität.**
`preview.go` (`widgetDefaultFontSizes`) und `widgets.js`
(`getPreviewFontSize`) melden für `widget_holidays` beide **13**, verifiziert
durch `TestWidgetDefaultFontSizesMatchFrontend`
(`widget_registration_test.go:478`), der jeden Dispatch-Typ auf beiden Seiten
explizit verlangt.

**AC22 — Regeltabellen-Stichtag.**
`widget_holidays.go` definiert `const holidayRulesAsOf = "2026-01-01"` mit einem
Kommentar, der die Quelle und den Rechtsstand nennt. `TestHolidayRuleTableStamp`
prüft, dass die Konstante existiert und ein parsbares Datum ist.

---

## Test-Anforderungen

Datei `server/internal/services/widget_holidays_test.go`.

Helper nach dem F7-Muster (`widget_progress_test.go:14-18`):

```go
func newHolidaysService(frozen time.Time) *PreviewService {
    s := &PreviewService{}
    s.now = func() time.Time { return frozen }
    return s
}
```

| # | Test | Deckt ab |
|---|---|---|
| 1 | `TestEasterSunday` | AC5 (Stützstellen + 201-Jahres-Invariante) |
| 2 | `TestBussUndBettag` | AC8 |
| 3 | `TestHolidaysPerState` | AC6, AC7 (Anzahlen, Zugehörigkeiten, Teilmengen-Invariante, 08.08. nirgends) |
| 4 | `TestDaysBetweenCalendarDays` | AC10 (46/45), AC11 (DST-Kontrolle) |
| 5 | `TestHolidaysYearRollover` | AC9 |
| 6 | `TestHolidaysCountdownWording` | AC11 |
| 7 | `TestHolidaysTimezoneHandling` | AC12 (inkl. ungültiger Zone, mit `mustLoadLocation`/Skip) |
| 8 | `TestHolidaysDefaultsAndInvalidInput` | AC14 |
| 9 | `TestHolidaysZeroValueServiceDoesNotPanic` | AC14 letzter Punkt |
| 10 | `TestHolidaysCharsetSafe` | AC16 |
| 11 | `TestHolidaysLayoutsRegistered` | Punkt 8 direkt |
| 12 | `TestHolidayRuleTableStamp` | AC22 |
| 13 | `TestHolidaysClockReadOnce` | AC13 (AST) |
| 14 | `TestHolidaysTestsUseFloat64Props` | AC15 (AST) |

In `widget_registration_test.go` ergänzen:

| # | Test | Deckt ab |
|---|---|---|
| 15 | `TestHolidaysCanvasPanelParity` | AC1, AC2, AC3 — Muster `TestProgressCanvasPanelParity` (`:350`) |

Automatisch mitlaufend, sobald der Dispatch-`case` steht (keine Änderung nötig):
`TestWidgetRegistrationCompleteness` (`:190`), `TestDispatchWidgetTypesFound`
(`:175`, die Untergrenze `len(types) < 8` bleibt gültig),
`TestWidgetDefaultFontSizesMatchFrontend` (`:478`), `TestDeadPlaceholderRegistry`
(`:279`).

**Gepinnter Golden-String.** Bei `goldenNow` (2026-07-20 12:00 CEST),
`state=DE`, `layout=next_countdown`, `timezone=Europe/Berlin`:

```
Tag der Deutschen Einheit
Sa, 03.10.2026 (in 75 Tagen)
```

Dieser String wird in `TestHolidaysCanvasPanelParity` als Literal geprüft — er
ist derselbe, der im Golden-PNG steht.

---

## Non-Goals

- **Keine** API-Anbindung, kein `openholidaysapi.org`, kein
  `feiertage-api.de`, kein Cache, kein `failCache`-Präfix, kein
  `safeFetchAllowlisted`. Siehe Architekturentscheidung.
- **Kein** globales Standort-/Bundesland-Setting in `settings.go`, `.env.example`
  oder der Settings-UI. Nur die widget-lokale `state`-Property.
  Querschnittsthema für **F6/F5**.
- **Keine** kommunalen Feiertage: Augsburger Friedensfest, Mariä Himmelfahrt in
  bayerischen Gemeinden, Fronleichnam in sächsischen/thüringischen Gemeinden.
  Das Widget hat keine Gemeindeauflösung und soll keine bekommen.
- **Keine** Ostersonntag-/Pfingstsonntag-Einträge (Begründung oben).
- **Keine** Schulferien, keine Brückentage, keine „Feiertag in anderen
  Ländern"-Funktion, kein Österreich/Schweiz.
- **Keine** „Ist heute ein Feiertag"-Anzeige in anderen Widgets (Clock,
  Calendar). Kein Eingriff in bestehende `fill*Content`-Funktionen.
- **Kein** eigener Grafikpfad, kein neuer `drawElement`-Zweig, keine Icons,
  keine Farbcodierung.
- **Keine** Auflösung des Font-Size-Duplikats `preview.go` ↔ `widgets.js` —
  nur beide Seiten konsistent auf 13 setzen.
- **Keine** i18n-Infrastruktur; deutsche Strings hartkodiert wie in `locale.go`.
- **Keine** Client-Änderungen (`client/`), keine neuen Dependencies, kein
  Build-Step.

**Diff-Budget: ≤ 400 Zeilen** ohne Tests, ohne Golden-PNGs, ohne
`holidays.json`. Grobe Aufteilung: Regeltabelle + Osterformel + Buß- und
Bettag ≈ 150, Content-/Layout-Logik ≈ 120, Registrierungspunkte 1–6 (JS/HTML)
≈ 90, Punkte 7–8 (Go) ≈ 25.

---

## Verifikation

**L1 — statisch**

```
cd server && gofmt -l . && go vet ./... && go test -count=1 ./...
```

Erwartung: `gofmt -l` leer, `go vet` still, alle Tests grün.
`-count=1` ist **Pflicht**: die Registrierungstests lesen Dateien
**außerhalb** des Go-Pakets (`static/js/*.js`, `templates/designer.html`), von
denen der Testcache nichts weiß — ein grüner Lauf aus dem Cache beweist bei
einer reinen Frontend-Änderung nichts.

**L2 — Registrierung und Parität**

```
cd server && go test -count=1 ./internal/services -run 'TestWidgetRegistrationCompleteness|TestWidgetDefaultFontSizesMatchFrontend|TestHolidaysCanvasPanelParity|TestDeadPlaceholderRegistry' -v
```

Erwartung: alle grün und **kein SKIP**. Ein übersprungener Registrierungstest
ist selbst ein Befund (`braceBlockAfter` macht `t.Skip` statt rot zu werden).

**L3 — Render und Palette**

```
cd server && go test -count=1 ./internal/services -run 'TestGoldenRender|TestPaletteExactness' -v
git diff --stat server/internal/services/testdata/golden/
```

Erwartung: im Diff erscheinen **nur** die zwei neuen `holidays__*.png`; die elf
bestehenden Golden-Dateien sind unverändert. Der Reviewer öffnet beide neuen
PNGs und prüft per Auge: alle drei Elemente lesbar, kein Tofu bei `ß`/`ä`,
keine Zeile abgeschnitten oder umgebrochen.

Regenerierung (**nur bewusst**, nie um einen roten Test zu beruhigen):

```
cd server && go test ./internal/services -run TestGoldenRender -update
```

**L4 — manuelle Canvas-Parität**

```
cd server && go run .
```

Widget aus der Palette ziehen, `state` durch alle 17 Werte und `layout` durch
alle vier schalten. Der Canvas-Text muss zeichengleich dem Panel-Preview
(`GET /preview`) entsprechen. Besonders prüfen: `SN` zeigt Buß- und Bettag,
`BY` zeigt Fronleichnam und Allerheiligen aber **nicht** Mariä Himmelfahrt.

**L5 — Review**

Der Reviewer prüft explizit:
(a) alle acht Registrierungspunkte,
(b) keine zweite Content-Formatierung in JS,
(c) `nowOrDefault()` genau einmal aufgerufen,
(d) `daysBetween` rechnet über UTC-Mitternachte, nicht über `24*time.Hour`,
(e) `"holidays"` steht in **allen vier** Listen: `goldenDesigns`,
    `goldenTZDesigns`, `ditherDesigns` und `testdata/designs/`,
(f) alle numerischen Test-Properties sind `float64(...)`.

---

## Risiken

| Risiko | Wirkung | Gegenmaßnahme / Rollback |
|---|---|---|
| Regeltabelle veraltet nach einer Gesetzesänderung | Ein Feiertag fehlt oder erscheint zu Unrecht in einem Land | `holidayRulesAsOf` macht das Alter sichtbar (AC22); Fix ist eine Zeile in der Tabelle plus Testzeile |
| `ditherDesigns`-Eintrag vergessen | Die Palettenprüfung läuft für das Design **nie**, AC18 ist vakuum-grün | AC17 nennt alle vier Listen einzeln; L5 (e) prüft sie explizit |
| Countdown über `24*time.Hour` gerechnet | An zwei Tagen im Jahr ein Tag daneben; auf dem Panel nicht auffällig, weil plausibel | AC11 DST-Kontrolle + verbindliche `daysBetween`-Regel über UTC-Mitternachte |
| Jahresübergang: nur `Y` statt `Y..Y+2` erzeugt | Ende Dezember leerer Content oder Countdown auf einen vergangenen Feiertag | AC9 mit vier Zeitpunkten, inkl. `count=10` am 31.12. |
| Test-Property als blankes `int` | Test ist grün, prüft aber den Default statt des Wertes — echter F7-Bug | AC15 als AST-Test, nicht als Konvention |
| `nowOrDefault()` zweimal gelesen | Torn Read über Mitternacht: Datum aus dem einen Tag, Countdown aus dem anderen | AC13 als AST-Test |
| Nicht-ASCII-Sonderzeichen (Gedankenstrich, Blockzeichen) schleichen sich ein | Tofu/Leerraum auf dem Panel, Canvas sieht korrekt aus | AC16 als harter Test, Latin-1-Grenze bei U+0100 |
| `DE`-Default wird als „Bundesland Deutschland" missverstanden | Nutzer sieht zu wenige Feiertage und hält es für einen Bug | Label im Dropdown lautet `Bundesweit (alle Länder)`, nicht `Deutschland`; steht an erster Stelle |
| tzdata fehlt zur Laufzeit | `time.LoadLocation` schlägt fehl → **stiller** Fallback auf Serverzeit; Countdown bis zu einen Tag daneben, ohne Fehler oder Log. Betrifft jedes Widget mit `timezone`-Property | Heute abgedeckt (`Dockerfile:31-32` setzt `ZONEINFO`); Tests skippen ehrlich über `goldenTZDesigns` und `mustLoadLocation`. Der offene Folge-Task `import _ "time/tzdata"` (~450 KB) bleibt offen und ist **nicht** Teil von F4 |
| Osterformel per Copy-Paste mit Vorzeichenfehler | Falsch, aber plausibel aussehend — Stützstellen allein fangen das nicht zuverlässig | AC5 Invariante über 201 Jahre: immer Sonntag, immer im Intervall 22.03.–25.04. |
| Beispielwerte im Spec statt aus der Regel abgeleitet | Falsche Zahlen wandern in die Implementierung | Jedes Datum und jede Tagesdifferenz in diesem Spec ist von Hand nachgerechnet und muss als Tabellentest gepinnt werden |

**Rollback:** `git revert` entfernt Widget, Golden-Design und beide PNGs
zusammen. Kein bestehender Code-Pfad wird verändert — der Dispatch bekommt
einen `case`, die zwei Font-Size-Tabellen je einen Eintrag, `layouts.go` je
einen Map-Key. Bestehende Golden-Dateien und alle anderen Widgets bleiben
unberührt.

---

## In Scope vs. verschoben

**In Scope (F4):**
- `widget_holidays` mit vier Layouts, 17 `state`-Werten, lokaler Berechnung
- Osterformel, Buß- und Bettag, vollständige Regeltabelle für alle 16 Länder
- alle acht Registrierungspunkte
- 15 Tests, Golden-Design in allen vier Listen, PNGs für beide Displays

**Verschoben (nicht F4):**
- globales Standort-/Bundesland-Setting → **F6/F5**, Querschnittsthema
- `import _ "time/tzdata"` → offener Folge-Task aus F7, unverändert offen
- Auflösung des Font-Size-Duplikats Go ↔ JS → unverändert offen
- Schulferien, Brückentage, Nachbarländer → kein Task, kein Bedarf belegt
- „Heute ist Feiertag"-Marker in Clock/Calendar → eigener Task, ändert
  bestehende Ausgaben und damit Golden-Dateien
