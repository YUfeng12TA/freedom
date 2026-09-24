From: https://github.com/webview/webview_go
Upstream commit: 6173450d4dd6 (v0.0.0-20240831120633)
License: MIT (see LICENSE)

Why this copy exists
------------------
Upstream hardcodes the CGO dependency name as `pkg-config: gtk+-3.0 webkit2gtk-4.0`.
Ubuntu 24.04 / Debian 13 removed the webkit2gtk-4.0 dev package (libsoup2 line) and ship
the API-compatible webkit2gtk-4.1 instead, so `go build` fails there with
"Package webkit2gtk-4.0 was not found".

What was changed (the only two deltas)
--------------------------------------
1. webview.go: dropped the hardcoded linux `pkg-config:` line (comment marks the spot).
2. webkit2_40.go / webkit2_41.go: the same directive, split by mutually exclusive build
   tags. Default stays 4.0 (unchanged behaviour on old distros and in CI artifacts);
   `-tags webkit2_41` selects 4.1.

Nothing else was touched: examples/ and webview_test.go were deleted, no code change.

Who flips the tag
-----------------
- repo/dev + CI: build.sh sources tools/webkit-env.sh (appends -tags=webkit2_41 to GOFLAGS)
- npm CLI: freedom-cli/lib/webkit.js (applyWebkitTags), used by `freedom shell build`
Detection is by pkg-config availability, so no user-facing flag is needed.

To refresh from upstream: copy the new tree over this directory, re-apply the two deltas
above, delete examples/ and webview_test.go, then re-sync the identical copy under
freedom-cli/templates/go/third_party/webview_go/ (it is what `freedom shell build` compiles).

webkitgtk-6.0 is NOT bridgeable this way (different C API: WebKitNetworkSession,
permission handlers); it needs an upstream port.
