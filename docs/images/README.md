# Screenshot artwork

This is an illustration of the shared Vue v2.2.9 frontend, captured in Chrome at 1440 × 900 against a loopback-only synthetic fixture. `internal/html/vue-ui-source.json` records the same frontend source commit, `f86943ebc764688c32bc4389f2276bc12682a90e`. It illustrates the bundled UI; the screenshot is not evidence of a running Go backend, a live installation, or hardware acceptance.

`vue-demo.jpg` is the retained flat source. Its SHA-256 is
`567da1017b9f5555ee8c0a5366627b33b8711974b815e98aac0ef424c968cde4`.
The README links the perspective hero to this source for readable detail.

`app-hero.png` applies only a geometric perspective projection, rounded outer
corners and a soft shadow on a neutral background. No interface text, controls,
charts or readings are generated or repainted. This artwork is documentation,
not test or release acceptance evidence.

## Regenerate

Install ImageMagick 7, then run from the repository root:

```sh
bash scripts/render-docs-hero.sh
```

The output is a static 1600 × 1050 PNG. The script checks source dimensions,
uses a temporary directory, strips metadata and replaces the hero only after a
successful render. Same source and ImageMagick version produce identical output;
resampling can differ across versions. Initially rendered with ImageMagick 7.1.2-31.

When replacing a screenshot, use synthetic or explicitly public data, record the
application version, review the complete flat image for private information,
and update this provenance and the README caption before regenerating.
