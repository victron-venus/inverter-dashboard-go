# Inverter Dashboard Go

[English](README.md) · [Русский](README.ru.md) · [Українська](README.uk.md) · [Deutsch](README.de.md) · [Français](README.fr.md) · [Nederlands](README.nl.md)

Webdashboard voor een Victron-installatie: een Go-server met de Vue-interface ingebouwd in het uitvoerbare bestand. Telemetrie komt rechtstreeks van de Cerbo via MQTT of via HTTPS inverter-gateway (IGW). Home Assistant kan gegevens en bediening voor ingestelde huishoudelijke apparaten toevoegen. Voor een kant-en-klaar uitvoerbaar bestand zijn Node.js en npm niet nodig.

![Architectuur: MQTT of de HTTPS-gateway levert gegevens aan de Go-server; browser, Home Assistant en privéopslag zijn afzonderlijke onderdelen.](docs/assets/architecture.svg)

De [geïllustreerde gids over het proces en de resultaten](docs/process-and-results.md) beschrijft de updatestappen, de verbeterde validatie van het HA-pollinterval en de Home-mapping met stabiele sleutels. Resultaten van broncodetests staan los van bewijs van een release, uitrol en controle in de browser.

## Functies

- Net, accu’s, zonneproductie, verbruik, water en elektrisch voertuig, voor zover de geïnstalleerde diensten deze gegevens leveren.
- Dagstatistieken, energie-intervallen, voorspellingen en tarieven van de controller. Ontbrekende gegevens worden niet vervangen door nulwaarden of een schijnbaar volledige historie.
- ESS-keuze, controllerschakelaars, watermodi, blijvende setpointoverride en tariefbewerking als de functie wordt ondersteund en de waarnemingen actueel zijn.
- Instelbare Home-knoppen, actieve huishoudelijke apparaten, afdekkingen, mediaspelers, scènes, numerieke waarden, sensoren en weer uit Home Assistant.
- Native waarschuwingen met de oorspronkelijke gebeurtenistijd en versleutelde Web Push-systeemmeldingen na uitdrukkelijke toestemming.
- Opgeslagen instellingen, HTTP/WebSocket-API, `/health` en Prometheus-metrieken via `/metrics`.

Dit is geen garantie voor volledige gelijkheid met Desktop of ondersteuning van alle hardware. Beschikbaarheid hangt af van bron, firmware, configuratie en actualiteit van de gegevens.

## Installatie en eerste start

Kies een specifieke versie voor uw platform bij [Releases](https://github.com/victron-venus/inverter-dashboard-go/releases). Controleer de gepubliceerde controlesommen en herkomstgegevens. Beta-, nightly- en RC-versies zijn voorvertoningen. De basisversie van de broncode staat in [VERSION](VERSION).

Voor het bouwen is Go volgens [go.mod](go.mod) nodig, momenteel 1.26.9:

```sh
git clone https://github.com/victron-venus/inverter-dashboard-go.git
cd inverter-dashboard-go
go mod download
go build -trimpath -ldflags "-X main.Version=$(cat VERSION)" -o inverter-dashboard .
./inverter-dashboard --version
cp config.yaml.example config.yaml
chmod 600 config.yaml
```

**Bewerk `config.yaml` voordat u start.** De voorbeelddomeinen werken bewust niet. Vul uw bron en portal-ID in en stel `DASHBOARD_SECRET` in via de afgeschermde omgeving van de dienst. Start daarna `./inverter-dashboard` vanuit de configuratiemap. Het programma leest het bestand uit de werkmap; er is geen optie `--config`.

Het voorbeeld luistert op `127.0.0.1:8080`. Gebruik een vertrouwde HTTPS-proxy voor toegang op afstand en Web Push. Rechtstreekse TLS vereist zowel `--ssl-cert` als `--ssl-key`. De container werkt in `/app`: het bestand hoort normaal op `/app/config.yaml`, niet alleen in de submap `/app/config`.

## Configuratie en opslag

De [configuratiereferentie](docs/configuration.md) beschrijft alle velden. Stel voor MQTT `mqtt.host` en `cerbo.portal_id` in. Een expliciete portal-ID maakt de eerste leesverzoeken mogelijk wanneer de broker stil is. IGW heeft een HTTPS-URL en de juiste toegangsgegevens nodig. Een lege YAML-waarde voor `mqtt.host` met ingeschakelde gateway kiest alleen IGW. Zijn beide bronnen ingesteld, dan heeft MQTT voorrang; bij uitval neemt IGW over en wordt herstel van MQTT periodiek gecontroleerd.

Voorrang verschilt per veld: YAML vervangt gewone MQTT-/webwaarden; expliciete `GATEWAY_*`-variabelen vervangen gateway-YAML. Daarna volgen CLI-opties en opgeslagen MQTT-/HA-verbindingsinstellingen. Verbindingswijzigingen vereisen een herstart.

Home-knoppen komen uitsluitend uit uw `homeassistant.switch_entities`. Een YAML-mapping met stabiele sleutels bewaart labels, de volgorde `order` en `enabled`. Een lege mapping (`{}`) geeft geen knoppen; onbekend betekent niet uitgeschakeld. Behoud uw eigen apparaten en toegangsgegevens bij updates in plaats van ze door voorbeelden te vervangen. Een huishoudelijk apparaat wordt alleen als actief getoond als zichtbaarheid is ingeschakeld en de toestand aangeeft dat het werkt.

Stel `INVERTER_DASHBOARD_SETTINGS_FILE` in voor blijvende instellingen, bijvoorbeeld `/var/lib/inverter-dashboard/settings/dashboard_settings.json`. Standaard wordt `dashboard_settings.json` in de werkmap gebruikt. Het bestand kan een HA-token bevatten, hoewel de API dit maskeert. Er is privéopslag nodig die beschrijfbaar is en een vervanging overleeft; een alleen-lezen Secret of tijdelijke map volstaat niet. Zichtbaarheidsinstellingen werken zonder herstart.

## Veilige bediening

Zonder `DASHBOARD_SECRET` is de ingebouwde authenticatie uitgeschakeld. Er zijn geen afzonderlijke gebruikersrollen voor alleen-lezen toegang. Gebruik bij voorkeur een Bearer-header voor de API; tokens in URL’s kunnen in de geschiedenis of toegangslogboeken terechtkomen. `/health`, `/metrics` en aangewezen statische bestanden zijn openbaar op applicatieniveau; beperk deze indien nodig bij de proxy.

Voor verzending controleert de server het toegestane doel, de huidige bron en de actualiteit van waarnemingen. Retained-MQTT-gegevens mogen worden getoond, maar geven geen nieuwe toestemming voor bediening. Een geaccepteerd verzoek bewijst geen fysieke uitvoering: de waargenomen apparaatstatus blijft bepalend.

**DRY is geen algemene veiligheidsblokkering.** Het instellen en stoppen van een blijvende setpointoverride staat volgens het controllerprotocol los van DRY. Test een installatie niet door apparatuur te schakelen of echte alarmen te bevestigen. Zie [bediening en beheer](docs/operations.md) en [SECURITY.md](SECURITY.md).

## Gebeurtenissen en Web Push

Een opnieuw ontvangen waarschuwing behoudt de oorspronkelijke tijd. Een ontbrekende of ongeldige tijd blijft onbekend. De interface vernieuwt de relatieve leeftijd zonder nieuwe berichten en na terugkeer naar het tabblad; de exacte lokale tijd en tijdzone blijven beschikbaar. Oplossen, bevestigen en dempen van een alarm zijn verschillende handelingen.

Web Push vereist HTTPS, een geschikte browser, toestemming van het besturingssysteem, `WEB_PUSH_ENABLED=true` en een absoluut `WEB_PUSH_DATA_DIR` op blijvende opslag. De privéopslag op de server wordt ondersteund op Linux/macOS; onder Windows is deze functie niet beschikbaar. Gebruik één proces per opslag en behoud sleutels en abonnementen bij updates. Verwijder ze niet om een afleverprobleem te proberen op te lossen.

Starten en opnieuw verbinden versturen oude gebeurtenissen niet opnieuw. Grenzen: 64 abonnementen, 1024 wachtende afleveringen, twee gelijktijdige verzendingen, maximaal drie pogingen en een levensduur van hoogstens vijf minuten per gebeurtenis. API-status `202` betekent in de wachtrij; providerstatus `2xx` betekent aangenomen door de provider, niet weergegeven door het besturingssysteem. [Meer informatie](docs/web-push.md).

## Problemen oplossen, interface en ontwikkeling

Opent de pagina niet, controleer dan luisteradres, DNS/VPN-route, TLS en authenticatie. Controleer bij onbruikbare Home-knoppen de daadwerkelijke HA-waarnemingen, niet alleen een verbindingsicoon. Controleer bij verdwenen instellingen pad, rechten en levensduur van het volume. Begin met [controles zonder apparatuur te bedienen](docs/troubleshooting.md). Publiceer geen tokens, Push-endpoints of privéconfiguratie.

Ingebouwd is de officiële Vue-versie **v2.2.7-beta.9**, bron `3d4a03fd3401334ac9e69f6d103caff41dc243ae`; het [herkomstbestand](internal/html/vue-ui-source.json) bevat metadata en hashes. Nieuwe vertalingen in het aparte Vue-project verschijnen pas na opname van een officieel artefact in een Go-binary. De taal van deze README bewijst niet dat de ingebouwde interface al vertaald is.

Voor bijdragen: [ontwikkelaarshandleiding](docs/development.md), [CONTRIBUTING](CONTRIBUTING.md) en `bash scripts/ci.sh`. De interface wordt ontwikkeld in [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue); wijzig geen ingebouwde gegenereerde bestanden. [Wiki](docs/wiki/Home.md) · [controlelijst](README-CHECKLIST.md) · [releasebeleid](RELEASING.md) · [licentie](LICENSE).
