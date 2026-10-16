# Process and results

[Home](Home.md) · [Configuration](Configuration.md) · [Development](Development.md)

The Go source, official frontend, backend release and running dashboard each need their own evidence. The diagram describes the update process; it does not mark a release or deployment as complete.

![Source, tests and review, official frontend, backend release, k3s and browser verification](https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main/docs/assets/update-process.svg)

A focused change is reproduced and tested in its owning repository. When the UI changes, verify the official Vue artifact and embed its exact bytes with a truthful receipt. Follow the release policy, deploy only when authorized while preserving private persistent state, and then inspect the browser through read-only checks. A green test is not proof of release; a ready Pod is not proof of correct UI or OS notification display.

## Polling validation result

The October 15, 2026 source review found that YAML floating-point inputs can turn into zero or excessively long ticker durations. The Go configuration guard now computes nanoseconds before conversion: NaN, values below one nanosecond, and values at or above the floating-point duration upper bound use the existing 12-second default. Valid fractional seconds remain valid: `0.25` seconds becomes 250 milliseconds.

![HA polling decision: keep a positive representable duration, otherwise use the 12-second default](https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main/docs/assets/polling-input-validation.svg)

The regression loads real YAML fixtures, checks each expected duration, and constructs/stops a ticker. It covers NaN, both infinities, zero, negative, subnanosecond, overflow and ordinary fractional inputs. Focused configuration and HA package race checks passed during the source change; this is not a benchmark or a claim of live deployment acceptance.

## Configuration and documentation result

Home controls come from a `homeassistant.switch_entities` mapping with stable keys and configured labels, order and enabled flags. `{}` means no buttons. Preserve your private inventory; examples are not a replacement for it. The six README languages, accurate Go build instructions, persistence guidance and editable diagrams make these boundaries explicit. New UI translations still require adoption of an official Vue artifact.

See the [full process and results guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/process-and-results.md) for the exact source, test cases and links. [Release policy](https://github.com/victron-venus/inverter-dashboard-go/blob/main/RELEASING.md) and the [operator guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/operations.md) remain authoritative. Never use physical commands or real alarm acknowledgement as a smoke test, and never publish credentials or push endpoints as evidence.
