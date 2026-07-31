---
layout: home

hero:
  name: flutter_go_bridge
  text: Go types, straight into Dart
  tagline: A Go → Dart/Flutter code generator for Gokit. No Flutter Native Assets, no package:flutter/services.dart — the generated code only needs the Dart SDK.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: What is it?
      link: /guide/
    - theme: alt
      text: GitHub
      link: https://github.com/star4277/flutter_go_bridge

features:
  - title: CST in, DCO out
    details: Dart → Go calls travel as real C wire structs; Go → Dart results come back as Dart_CObject. Types that neither can express yet fall back to a pure-Dart standard codec — never to the Flutter SDK.
    link: /concepts/serialization
    linkText: Serialization strategy
  - title: One stable ABI
    details: Every call is dispatched through a fixed set of exported symbols such as fgb_cst and fgb_init. Adding a Go function does not add a C symbol, so your CMake and Gokit setup never changes.
    link: /concepts/stable-abi
    linkText: Exported symbols
  - title: Streams from a plain channel
    details: Add a chan<- T parameter and Dart gets a Stream<T>. The generated code creates the channel, drains it, and closes it when your function returns. Add a context.Context and cancellation is wired up too.
    link: /reference/stream
    linkText: Stream reference
  - title: Structs, interfaces, embedding
    details: Translatable structs become Dart value classes, embedded fields become Dart inheritance, and named interfaces become abstract interface class with implements. Anything that cannot travel becomes a GoOpaque handle.
    link: /reference/structs-interfaces
    linkText: Structs and interfaces
  - title: Dart closures as Go callbacks
    details: A Go function can take a native func(...) parameter and Dart passes a closure — sync or async, both are awaited by the runtime before the result goes back to Go.
    link: /reference/callbacks
    linkText: Callback reference
  - title: Watch, run, hot reload
    details: generate --watch regenerates on save. run drives flutter run for you — hot reload for Dart edits, and a full process restart for Go edits, because a dynamic library cannot be swapped in place.
    link: /guide/dev-server
    linkText: Dev server
---

<div class="fgb-flow">
  <div class="fgb-flow-step fgb-flow-step--go">
    <h3>Your Go package</h3>
    <p>Ordinary Go. Mark a function <code>//fgb:async</code> when you want a Dart <code>Future</code>, and leave everything else alone.</p>
  </div>
  <div class="fgb-flow-arrow" aria-hidden="true">→</div>
  <div class="fgb-flow-step fgb-flow-step--gen">
    <h3>flutter_go_bridge_codegen</h3>
    <p>Reads types with <code>go/packages</code>, picks a codec per call, and writes one <code>bridge_generated.go</code> plus a mirrored Dart tree.</p>
  </div>
  <div class="fgb-flow-arrow" aria-hidden="true">→</div>
  <div class="fgb-flow-step fgb-flow-step--dart">
    <h3>Your Dart API</h3>
    <p>One Dart file per Go source file, named parameters throughout, and every FFI detail confined to <code>bridge_generated.dart</code>.</p>
  </div>
</div>
