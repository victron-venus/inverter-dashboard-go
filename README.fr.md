# Inverter Dashboard Go

[English](README.md) · [Русский](README.ru.md) · [Українська](README.uk.md) · [Deutsch](README.de.md) · [Français](README.fr.md) · [Nederlands](README.nl.md)

Tableau de bord web pour une installation Victron : un serveur Go avec l’interface Vue intégrée au binaire. La télémétrie provient directement du Cerbo par MQTT ou d’inverter-gateway (IGW) en HTTPS. Home Assistant peut ajouter les appareils et capteurs domestiques configurés. Node.js et npm ne sont pas nécessaires pour exécuter le binaire.

![Architecture : MQTT ou la passerelle HTTPS alimentent le serveur Go ; navigateur, Home Assistant et stockage privé sont des composants distincts.](docs/assets/architecture.svg)

Le [guide illustré du processus et des résultats](docs/process-and-results.md) explique les étapes de mise à jour, la correction de la validation de l’intervalle HA et les règles du dictionnaire Home à clés stables. Les résultats des tests du code sont distingués des preuves de publication, de déploiement et de vérification dans le navigateur.

## Fonctionnalités

- Réseau, batteries, production solaire, consommation, eau et véhicule électrique, selon les services et données disponibles dans l’installation.
- Statistiques quotidiennes, intervalles d’énergie, prévisions et tarifs fournis par le contrôleur. Les données absentes ne deviennent ni des zéros ni un historique prétendument complet.
- Choix ESS, commutateurs du contrôleur, modes d’eau, consigne forcée persistante et édition des tarifs lorsque les capacités et observations récentes le permettent.
- Boutons Home configurables, appareils actifs, volets, lecteurs multimédias, scènes, valeurs numériques, capteurs et météo Home Assistant.
- Alertes natives conservant l’heure d’origine et notifications système Web Push chiffrées après consentement explicite.
- Paramètres persistants, API HTTP/WebSocket, contrôle `/health` et métriques Prometheus `/metrics`.

Cela ne garantit pas une équivalence totale avec Desktop ni la prise en charge de tout matériel. La disponibilité dépend de la source, du micrologiciel, de la configuration et de la fraîcheur des données.

## Installation et premier démarrage

Choisissez une version précise pour votre plateforme dans [Releases](https://github.com/victron-venus/inverter-dashboard-go/releases), puis vérifiez les sommes de contrôle et les preuves de provenance publiées. Les versions beta, nightly et RC sont des préversions. La version de base du code figure dans [VERSION](VERSION).

La compilation utilise Go tel que déclaré dans [go.mod](go.mod), actuellement 1.26.9 :

```sh
git clone https://github.com/victron-venus/inverter-dashboard-go.git
cd inverter-dashboard-go
go mod download
go build -trimpath -ldflags "-X main.Version=$(cat VERSION)" -o inverter-dashboard .
./inverter-dashboard --version
cp config.yaml.example config.yaml
chmod 600 config.yaml
```

**Modifiez `config.yaml` avant de démarrer.** Les domaines d’exemple ne sont volontairement pas utilisables. Renseignez votre source et votre identifiant de portail, puis fournissez `DASHBOARD_SECRET` dans l’environnement privé du service. Lancez ensuite `./inverter-dashboard` depuis le répertoire de configuration. Le fichier est recherché dans le répertoire de travail ; il n’existe pas d’option `--config`.

L’exemple écoute sur `127.0.0.1:8080`. Utilisez un proxy HTTPS de confiance pour l’accès distant et Web Push. Le TLS natif nécessite à la fois `--ssl-cert` et `--ssl-key`. Le conteneur travaille dans `/app` : le fichier doit normalement être monté dans `/app/config.yaml`, pas seulement dans le sous-répertoire `/app/config`.

## Configuration et persistance

Consultez la [référence complète](docs/configuration.md). Pour MQTT, définissez `mqtt.host` et `cerbo.portal_id`. Un portail explicite permet les premières lectures même si le broker reste silencieux. Pour IGW, configurez une URL HTTPS et les identifiants requis. Un `mqtt.host` vide en YAML avec la passerelle activée sélectionne IGW seul. Si les deux sources sont configurées, MQTT est prioritaire ; IGW prend le relais en cas de panne, avec vérification périodique du retour de MQTT.

La priorité dépend du champ : YAML remplace les valeurs MQTT/web ordinaires ; les variables `GATEWAY_*` explicites remplacent le bloc YAML de la passerelle ; viennent ensuite les options CLI et les paramètres de connexion MQTT/HA enregistrés. Un changement de connexion nécessite un redémarrage.

Les boutons Home proviennent exclusivement de votre `homeassistant.switch_entities`. Un dictionnaire YAML à clés stables conserve les libellés, l’ordre `order` et `enabled`. Un dictionnaire vide (`{}`) ne crée aucun bouton ; un état inconnu n’est pas un état éteint. Préservez vos appareils et identifiants lors des mises à jour au lieu de les remplacer par les exemples. Un appareil domestique n’apparaît actif que si sa visibilité est activée et son état indique un fonctionnement.

Pour conserver les paramètres, définissez `INVERTER_DASHBOARD_SETTINGS_FILE`, par exemple `/var/lib/inverter-dashboard/settings/dashboard_settings.json`. Par défaut, le fichier est `dashboard_settings.json` dans le répertoire de travail. Il peut contenir un jeton HA, même si l’API le masque. Il faut un volume privé, persistant et accessible en écriture ; un Secret en lecture seule ou un répertoire temporaire ne suffit pas. Les paramètres de visibilité s’appliquent sans redémarrage.

## Sécurité des commandes

Sans `DASHBOARD_SECRET`, l’authentification intégrée est désactivée. Il n’existe pas de rôle utilisateur distinct pour la lecture seule. Préférez l’en-tête Bearer pour l’API : un jeton dans l’URL peut être enregistré dans l’historique ou les journaux. `/health`, `/metrics` et les ressources statiques désignées sont publics au niveau de l’application ; limitez-les au niveau du proxy si nécessaire.

Avant l’envoi, le serveur vérifie la cible autorisée, la source courante et la fraîcheur des observations. Les messages MQTT retained peuvent alimenter l’affichage, mais n’accordent pas une nouvelle autorisation de commande. Une requête acceptée ne prouve pas son exécution physique : l’état réel de l’appareil reste la référence.

**DRY n’est pas un verrou général.** L’activation et l’arrêt de la consigne forcée persistante sont indépendants de DRY selon le protocole du contrôleur. Ne validez pas une installation en actionnant un appareil ou en acquittant une alarme réelle. Voir [l’exploitation](docs/operations.md) et [SECURITY.md](SECURITY.md).

## Événements et Web Push

Une alerte reçue de nouveau conserve son heure d’origine. Une heure absente ou invalide reste inconnue. L’interface recalcule l’âge sans nouveau message et au retour dans l’onglet ; l’heure locale exacte et le fuseau restent consultables. Résolution, acquittement et silence d’une alarme sont des opérations distinctes.

Web Push nécessite HTTPS, un navigateur compatible, l’autorisation du système, `WEB_PUSH_ENABLED=true` et un `WEB_PUSH_DATA_DIR` absolu sur stockage persistant. Le stockage privé côté serveur est pris en charge sous Linux/macOS ; la fonction reste indisponible sous Windows. Utilisez un seul processus par stockage et conservez clés et abonnements lors des mises à jour. Ne les supprimez pas pour tenter de réparer une livraison.

Le démarrage et les reconnexions ne renvoient pas l’historique. Limites : 64 abonnements, 1024 livraisons en attente, deux envois simultanés, trois tentatives au maximum et cinq minutes de durée de vie de l’événement. Le code API `202` signifie mise en file ; un `2xx` du fournisseur signifie acceptation par celui-ci, pas affichage par le système. [Détails](docs/web-push.md).

## Diagnostic, interface et développement

Si la page ne s’ouvre pas, vérifiez l’adresse d’écoute, le routage DNS/VPN, TLS et l’authentification. Pour des boutons Home indisponibles, vérifiez les observations HA réelles plutôt qu’un simple indicateur de connexion. Pour des paramètres perdus, vérifiez chemin, droits et persistance du volume. Commencez par les [contrôles sans action sur le matériel](docs/troubleshooting.md) ; ne publiez ni jetons, ni endpoints Push, ni configuration privée.

L’interface intégrée provient de l’artefact officiel **Vue v2.2.8** (version du paquet `2.2.8`, source `ff3865d373cc3819b83db0328d9855a2bdec9859`). La [preuve de provenance](internal/html/vue-ui-source.json) contient les métadonnées et empreintes. Les fichiers sont identiques octet pour octet à la RC validée `v2.2.8-rc.1` (exécution de compilation `38861405495`). De nouvelles traductions dans le dépôt Vue n’apparaissent dans Go qu’après l’intégration d’un nouvel artefact officiel. La langue du README ne prouve pas la traduction de l’interface déjà embarquée.

Pour contribuer : [guide développeur](docs/development.md), [CONTRIBUTING](CONTRIBUTING.md) et `bash scripts/ci.sh`. L’interface se développe dans [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue) ; ne modifiez pas les fichiers intégrés générés. [Wiki](docs/wiki/Home.md) · [liste de vérification](README-CHECKLIST.md) · [règles de publication](RELEASING.md) · [licence](LICENSE).
