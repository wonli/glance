# Upstream

This directory contains the production Go sources from
[`github.com/gogpu/systray`](https://github.com/gogpu/systray), version `v0.3.0`.
The upstream MIT license is preserved in `LICENSE`.

The local patch adds checked constructors and setup methods so desktop startup
can refuse close-to-tray residency when native icon registration fails. Keep
this copy limited to upstream source plus those small compatibility-preserving
changes; do not add unrelated platform behavior here.
