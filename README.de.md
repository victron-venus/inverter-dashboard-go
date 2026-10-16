# Inverter Dashboard Go

[English](README.md) · [Русский](README.ru.md) · [Українська](README.uk.md) · [Deutsch](README.de.md) · [Français](README.fr.md) · [Nederlands](README.nl.md)

Weboberfläche für eine Victron-Anlage: ein Go-Server mit eingebetteter Vue-Oberfläche. Telemetrie kommt direkt vom Cerbo über MQTT oder über ein HTTPS-Inverter-Gateway (IGW). Home Assistant kann Daten und Bedienelemente für konfigurierte Haushaltsgeräte ergänzen. Zum Ausführen einer fertigen Binärdatei werden weder Node.js noch npm benötigt.

![Architektur: MQTT oder HTTPS-Gateway liefern Daten an den Go-Server; Browser, Home Assistant und privater Speicher sind getrennte Komponenten.](docs/assets/architecture.svg)

Der [bebilderte Leitfaden zu Ablauf und Ergebnissen](docs/process-and-results.md) erklärt die Aktualisierungsschritte, die korrigierte Prüfung des HA-Abfrageintervalls und die Home-Zuordnung mit stabilen Schlüsseln. Ergebnisse der Quellcodetests werden von Release-, Bereitstellungs- und Browsernachweisen getrennt.

## Funktionen

- Netz, Batterien, PV-Erzeugung, Verbrauch, Wasser und Elektrofahrzeug, soweit die installierten Dienste entsprechende Daten liefern.
- Tagesstatistik, Energieintervalle, Prognose und Tarife vom Controller. Fehlende Daten werden nicht zu Nullwerten oder einer vermeintlich vollständigen Historie.
- ESS-Auswahl, Controller-Schalter, Wassermodi, dauerhafte Sollwertübersteuerung und Tarifbearbeitung bei unterstützter Funktion und aktuellen Beobachtungen.
- Konfigurierbare Home-Schaltflächen, aktive Haushaltsgeräte, Abdeckungen, Mediaplayer, Szenen, Zahlenwerte, Sensoren und Wetter aus Home Assistant.
- Native Warnungen mit ursprünglicher Ereigniszeit sowie verschlüsselte Web-Push-Benachrichtigungen nach ausdrücklicher Zustimmung.
- Gespeicherte Einstellungen, HTTP/WebSocket-API, `/health` und Prometheus-Metriken unter `/metrics`.

Dies garantiert weder vollständige Desktop-Funktionsgleichheit noch Unterstützung jeder Firmware. Verfügbarkeit hängt von Quelle, Konfiguration und Datenaktualität ab.

## Installation und erster Start

Wählen Sie eine konkrete Version für Ihre Plattform unter [Releases](https://github.com/victron-venus/inverter-dashboard-go/releases). Prüfen Sie veröffentlichte Prüfsummen und Herkunftsnachweise. Beta-, Nightly- und RC-Versionen sind Vorabversionen. Die Basisversion des Quellcodes steht in [VERSION](VERSION).

Zum Bauen ist die Go-Version aus [go.mod](go.mod) erforderlich, derzeit 1.26.9:

```sh
git clone https://github.com/victron-venus/inverter-dashboard-go.git
cd inverter-dashboard-go
go mod download
go build -trimpath -ldflags "-X main.Version=$(cat VERSION)" -o inverter-dashboard .
./inverter-dashboard --version
cp config.yaml.example config.yaml
chmod 600 config.yaml
```

**Bearbeiten Sie `config.yaml` vor dem Start.** Die Beispieldomains sind absichtlich nicht betriebsbereit. Tragen Sie Ihre Quelle und Portal-ID ein und setzen Sie `DASHBOARD_SECRET` über die geschützte Dienstumgebung. Starten Sie anschließend `./inverter-dashboard` im Konfigurationsverzeichnis. Die Datei wird im Arbeitsverzeichnis gesucht; eine Option `--config` gibt es nicht.

Das Beispiel lauscht auf `127.0.0.1:8080`. Für Fernzugriff und Web Push ist ein vertrauenswürdiger HTTPS-Proxy vorgesehen. Direktes TLS benötigt sowohl `--ssl-cert` als auch `--ssl-key`. Im Container ist `/app` das Arbeitsverzeichnis: Die Datei gehört normalerweise nach `/app/config.yaml`, nicht lediglich in ein Verzeichnis `/app/config`.

## Konfiguration und Speicherung

Die [Konfigurationsreferenz](docs/configuration.md) beschreibt alle Felder. Für MQTT setzen Sie `mqtt.host` und `cerbo.portal_id`. Eine feste Portal-ID ermöglicht erste Leseanforderungen auch bei einem stillen Broker. Für IGW konfigurieren Sie HTTPS-URL und Zugangsdaten. Ein leerer YAML-Wert für `mqtt.host` zusammen mit aktiviertem Gateway wählt reinen IGW-Betrieb. Bei zwei Quellen wird MQTT bevorzugt; bei Ausfall übernimmt IGW, während die MQTT-Wiederherstellung regelmäßig geprüft wird.

Die Priorität hängt vom Feld ab: YAML überschreibt gewöhnliche MQTT-/Web-Werte; explizite `GATEWAY_*`-Variablen überschreiben Gateway-YAML. Danach folgen CLI-Werte und gespeicherte MQTT-/HA-Verbindungseinstellungen. Verbindungsänderungen benötigen einen Neustart.

Home-Schaltflächen stammen ausschließlich aus Ihrer `homeassistant.switch_entities`-Zuordnung. Eine YAML-Zuordnung mit stabilen Schlüsseln bewahrt Beschriftung, Reihenfolge (`order`) und `enabled`. Eine leere Zuordnung (`{}`) erzeugt keine Schaltflächen; unbekannter Zustand bedeutet nicht ausgeschaltet. Bewahren Sie beim Aktualisieren Ihre vorhandenen Geräte und Zugangsdaten, statt sie durch Beispiele zu ersetzen. Haushaltsgeräte werden nur bei aktivierter Sichtbarkeit und tatsächlichem Betriebszustand als aktiv angezeigt.

Setzen Sie `INVERTER_DASHBOARD_SETTINGS_FILE` für dauerhafte Einstellungen, etwa auf `/var/lib/inverter-dashboard/settings/dashboard_settings.json`. Standard ist `dashboard_settings.json` im Arbeitsverzeichnis. Die Datei kann ein HA-Token enthalten, obwohl die API es maskiert. Sie benötigt geschützten, beschreibbaren, persistenten Speicher; ein schreibgeschütztes Secret oder temporäres Verzeichnis reicht nicht. Sichtbarkeitseinstellungen wirken ohne Neustart.

## Sicher bedienen

Ohne `DASHBOARD_SECRET` ist die eingebaute Authentifizierung aus. Es gibt keine getrennten Benutzerrollen für reinen Lesezugriff. API-Clients sollten einen Bearer-Header verwenden; URL-Token können in Verlauf oder Zugriffsprotokollen landen. `/health`, `/metrics` und ausgewählte statische Ressourcen sind auf Anwendungsebene öffentlich und gegebenenfalls am Proxy einzuschränken.

Vor dem Senden prüft der Server das konfigurierte Ziel, die aktuelle Quelle und die Frische der Beobachtung. Retained-MQTT-Daten dürfen angezeigt werden, erteilen aber keine neue Steuerberechtigung. Ein angenommener Auftrag beweist keine physische Ausführung: Maßgeblich bleibt der beobachtete Gerätezustand.

**DRY ist keine allgemeine Sicherheitssperre.** Setzen und Löschen einer dauerhaften Sollwertübersteuerung erfolgen laut Controller-Protokoll unabhängig von DRY. Prüfen Sie eine Installation nicht durch Geräteschaltungen oder das Bestätigen echter Alarme. Details stehen in [Betrieb und Sicherheit](docs/operations.md) sowie [SECURITY.md](SECURITY.md).

## Ereignisse und Web Push

Ein erneut empfangenes Ereignis behält seine ursprüngliche Zeit. Fehlende oder ungültige Zeiten bleiben unbekannt. Die Oberfläche aktualisiert das relative Alter ohne neue Nachrichten und bei Rückkehr zum Tab; die genaue lokale Zeit mit Zeitzone bleibt einsehbar. Auflösung, Quittierung und Stummschaltung eines Alarms sind unterschiedliche Vorgänge.

Web Push benötigt HTTPS, einen unterstützten Browser, die OS-Berechtigung sowie `WEB_PUSH_ENABLED=true` und ein absolutes `WEB_PUSH_DATA_DIR` auf persistentem Speicher. Der private serverseitige Speicher wird unter Linux/macOS unterstützt; unter Windows bleibt diese Funktion nicht verfügbar. Verwenden Sie nur einen Prozess pro Speicher und erhalten Sie Schlüssel und Abonnements bei Updates. Löschen ist keine geeignete Fehlerbehebung.

Start und Wiederverbindung versenden alte Ereignisse nicht erneut. Grenzen: 64 Abonnements, 1024 ausstehende Zustellungen, zwei parallele Sendungen, bis zu drei Versuche und höchstens fünf Minuten Ereignislebensdauer. API-Status `202` bedeutet eingereiht; Provider-Status `2xx` bedeutet beim Provider angenommen, nicht vom Betriebssystem angezeigt. [Weitere Einzelheiten](docs/web-push.md).

## Fehlerbehebung, Oberfläche und Entwicklung

Öffnet sich die Seite nicht, prüfen Sie Bind-Adresse, DNS/VPN-Route, TLS und Authentifizierung. Sind Home-Schaltflächen nicht bedienbar, prüfen Sie echte HA-Beobachtungen statt nur eines Verbindungssymbols. Gehen Einstellungen verloren, prüfen Sie Pfad, Rechte und Speicherlebensdauer. Beginnen Sie mit [Prüfungen ohne Gerätesteuerung](docs/troubleshooting.md). Veröffentlichen Sie keine Token, Push-Endpunkte oder privaten Konfigurationsdateien.

Eingebettet ist die offizielle Vue-Version **v2.2.8-beta.2**, Quellstand `f57b65532acf6b4142de52b607bca9cfe1ef05c0`; der [Herkunftsnachweis](internal/html/vue-ui-source.json) enthält Metadaten und Hashes. Neue Übersetzungen im separaten Vue-Repository erscheinen erst nach Übernahme eines offiziellen Artefakts in einer Go-Binärdatei. Die README-Sprache belegt keine bereits ausgelieferte Übersetzung der Oberfläche.

Für Beiträge: [Entwicklerhandbuch](docs/development.md), [CONTRIBUTING](CONTRIBUTING.md) und `bash scripts/ci.sh`. Die Oberfläche wird in [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue) entwickelt; generierte eingebettete Dateien nicht manuell bearbeiten. [Wiki](docs/wiki/Home.md) · [Prüfliste](README-CHECKLIST.md) · [Release-Regeln](RELEASING.md) · [Lizenz](LICENSE).
