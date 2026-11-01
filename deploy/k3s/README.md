# Deploy inverter-dashboard-go on k3s

These manifests are installation examples, not a record of the current cluster.
The `mp` node selector, `traefik-mp` ingress class, hostnames and `latest` image
must be reviewed for the target environment. The repository does not provision
the ingress controller, DNS, certificates or private configuration.

## Prepare a private deployment

Keep installation-specific files outside the repository, for example in
`~/.config/victron/inverter-dashboard-go/k3s/`. Protect the directory with mode
0700 and credential-bearing files with mode 0600. Copy and edit the manifests
there; do not overwrite an existing working configuration with these examples.
Preserve the local Home entity inventory, labels, ordering and disabled entries.

`02-secret.example.yaml` illustrates the two expected Secret shapes. It is
deliberately **excluded from `kustomization.yaml`**: applying the public base must
not overwrite working credentials with `replace-me`. Provision and maintain
`inverter-dashboard-go-gateway` and `inverter-dashboard-go-config` explicitly
through the installation's private secret-management process. The Deployment
continues to reference them; the latter is mounted at `/app/config.yaml`.
Optional Kubernetes references allow a pod to start without these Secrets; they
do not prove that gateway or HA authentication is configured correctly.

Before exposing the dashboard, supply `DASHBOARD_SECRET` through a private Secret
environment reference, serve it over HTTPS and restrict ingress to intended
users. An unset secret disables the application's shared-secret authentication,
including for command and notification APIs. The gateway bearer token protects
the gateway connection; it does not authenticate dashboard clients. See
[authentication](../../docs/configuration.md#http-authentication-and-storage).

Select a verified release image before applying. Follow
[release operations](../../docs/release-workflow.md) to import the published,
checksum-verified OCI archive without rebuilding it. Replace `newTag: latest` in
its private kustomization with the exact registry digest, for example:

```yaml
images:
  - name: alvit/inverter-dashboard-go
    newName: alvit/inverter-dashboard-go
    digest: sha256:<verified-registry-digest>
```

The digest above is a placeholder, not a deployable value. For beta/RC testing,
use the corresponding verified candidate archive and an isolated qualification
deployment. A successful candidate does not move production or `latest` by itself.
Stable promotion must reuse the accepted RC bytes.

## Preserve settings and notification identity

The public base does not allocate persistent storage. In the private deployment,
mount durable storage and set `INVERTER_DASHBOARD_SETTINGS_FILE` to a private file
such as `/var/lib/inverter-dashboard/settings/dashboard_settings.json`.
If Web Push is enabled, set `WEB_PUSH_DATA_DIR` to a separate private directory,
such as `/var/lib/inverter-dashboard/notifications`, with mode 0700 and files
mode 0600. Preserve both across image updates; the push store contains the VAPID
identity, subscriptions and pending delivery state.

Provision the mounted settings and push paths for the container's effective
runtime UID/GID; the image runs as the non-root `app` user. The settings parent
must be writable, and the 0700 push directory and its 0600 files must be owned
by that runtime user. A root-owned 0700 directory remains inaccessible even on
a writable volume. Verify ownership against the selected image and deployment
security context before starting the pod.

Use one replica and the `Recreate` strategy when sharing this store. An emptyDir
or the container writable layer does not preserve it during replacement. Keep
`SELF_UPDATE_ENABLED` disabled and update the verified image instead. See
[configuration](../../docs/configuration.md) and [Web Push](../../docs/web-push.md).

## Telemetry and controls

The examples configure IGW-only operation. Supported source selection is:

1. Configured, reachable MQTT is preferred.
2. Configured IGW provides failover while MQTT recovery is probed.
3. With MQTT disabled, configured IGW is the sole source.

Review the **effective** configuration: YAML MQTT fields and persisted connection
settings can override environment defaults. For gateway-only YAML, use
`mqtt: {host: "", port: 0}` and configure the gateway; remove any conflicting
persisted MQTT override. The configuration reference describes precedence.

IGW polling uses `GET /v1/snapshot`. Supported IGW commands include controller
toggles, dry-run, ESS mode, setpoint override, electricity tariff, native water
mode, alarm silence and notification acknowledgement. Controls still require
supported capabilities and current observations. A rejected command must not
be retried blindly, and an HTTP acknowledgement does not establish that a
physical device has reached the requested state.

The gateway URL must be absolute HTTPS without userinfo, query or fragment.
Existing path prefixes are supported. TLS verification stays enabled and all
redirects are rejected. Use the final endpoint and ensure cluster DNS resolves
its certificate hostname correctly.

Native IGW normally uses its bearer token. The example Deployment explicitly
sets empty `GATEWAY_ACCESS_CLIENT_ID` and `GATEWAY_ACCESS_CLIENT_SECRET`, overriding
inherited or YAML values. For a Cloudflare Access endpoint, change those entries
to references to the complete private client-id/client-secret pair. Never copy
credentials into the public manifests, commands, logs or release evidence.

## Review, apply and verify

Choose the actual context and private deployment directory. First render and
review that installation's manifests, then apply through its normal deployment
process. Do not apply `02-secret.example.yaml` with placeholder values.

```bash
kubectl --context "$KUBE_CONTEXT" kustomize "$DEPLOY_DIR"
kubectl --context "$KUBE_CONTEXT" apply -k "$DEPLOY_DIR"
kubectl --context "$KUBE_CONTEXT" -n inverter-dashboard-go rollout status deployment/inverter-dashboard-go
kubectl --context "$KUBE_CONTEXT" -n inverter-dashboard-go get pods,ingress -o wide
```

Secret values injected into the environment and the `/app/config.yaml` `subPath`
mount do not refresh in an existing pod. After changing either Secret, if an
image or pod-template update has not already replaced the pod, restart it
explicitly and wait for readiness:

```bash
kubectl --context "$KUBE_CONTEXT" -n inverter-dashboard-go rollout restart deployment/inverter-dashboard-go
kubectl --context "$KUBE_CONTEXT" -n inverter-dashboard-go rollout status deployment/inverter-dashboard-go
```

Rendering may contain secrets if the private overlay generates them: inspect it
locally and do not attach that output to public issues or CI logs. Before updating
production, retain the prior image digest and compatible private configuration
for rollback. Do not reset the notification store during rollback.

Read-only smoke checks should confirm `/`, `/health`, `/api/state`, the intended
ingress/TLS hostname and the pod's actual image ID. Verify version/provenance and
fresh selected-source telemetry separately: HTTP 200 alone does not establish
either. Compare the embedded frontend identity as well as the backend version;
see [browser acceptance](../../docs/browser-compatibility.md).

Use synthetic events and local mock services for candidate command/notification
qualification. Production smoke checks do not require toggling an inverter,
pump or HA entity. Native OS notification delivery requires a separately
observed browser test; a queued/provider-accepted response is not delivery proof.
