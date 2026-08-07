import 'dart:async';
import 'dart:convert';
import 'dart:js_interop';
import 'dart:js_interop_unsafe';

import 'package:flutter/services.dart';

const _assetPackage = 'REPLACE_ME_GO_MOD_NAME';
const _assetRoot = 'packages/$_assetPackage/assets/wasm';

final class FgbWasmManifest {
  FgbWasmManifest._(this.libraryName, this.wasmAsset, this.wasmExecAsset);

  final String libraryName;
  final String wasmAsset;
  final String wasmExecAsset;

  static FgbWasmManifest parse(String raw) {
    final value = jsonDecode(raw);
    if (value is! Map) {
      throw const FormatException('fgb_wasm_manifest.json must contain an object');
    }
    if (value['target'] != 'web' || value['schema_version'] != 1) {
      throw const FormatException('unsupported Gokit Web Wasm manifest');
    }
    final libraryName = value['library_name'];
    final artifacts = value['artifacts'];
    if (libraryName is! String || artifacts is! Map) {
      throw const FormatException('Web Wasm manifest is missing library_name or artifacts');
    }
    final wasm = artifacts.keys.whereType<String>().firstWhere(
      (key) => key.endsWith('.wasm'),
      orElse: () => throw const FormatException('Web Wasm manifest has no .wasm artifact'),
    );
    if (!artifacts.containsKey('wasm_exec.js')) {
      throw const FormatException('Web Wasm manifest has no wasm_exec.js artifact');
    }
    return FgbWasmManifest._(
      libraryName,
      '$_assetRoot/$wasm',
      '$_assetRoot/wasm_exec.js',
    );
  }
}

final class FgbWasmLoader {
  static Future<void> ensureReady({AssetBundle? bundle}) async {
    final assets = bundle ?? rootBundle;
    final manifest = FgbWasmManifest.parse(
      await assets.loadString('$_assetRoot/fgb_wasm_manifest.json'),
    );
    await _loadScript(manifest.wasmExecAsset);
    final goConstructor = globalContext.getProperty<JSFunction?>('Go'.toJS);
    if (goConstructor == null) {
      throw StateError('wasm_exec.js did not register the Go constructor');
    }
    final go = goConstructor.callAsConstructor();
    final fetch = globalContext.getProperty<JSFunction>('fetch'.toJS);
    final response = fetch.callAsFunction(null, manifest.wasmAsset.toJS);
    final webAssembly = globalContext.getProperty<JSObject>('WebAssembly'.toJS);
    final instantiate = webAssembly.getProperty<JSFunction>(
      'instantiateStreaming'.toJS,
    );
    final importObject = go.getProperty<JSObject>('importObject'.toJS);
    final result = await (instantiate.callAsFunction(null, response, importObject)
            as JSPromise<JSAny?>)
        .toDart;
    if (result is! JSObject) {
      throw StateError('WebAssembly.instantiateStreaming returned no instance');
    }
    final instance = result.getProperty<JSObject>('instance'.toJS);
    final run = go.getProperty<JSFunction>('run'.toJS);
    run.callAsFunction(go, instance);
    await _waitForBridge(manifest.libraryName);
  }

  static Future<void> _loadScript(String asset) async {
    final document = globalContext.getProperty<JSObject>('document'.toJS);
    final createElement = document.getProperty<JSFunction>('createElement'.toJS);
    final script = createElement.callAsFunction(document, 'script'.toJS);
    if (script is! JSObject) {
      throw StateError('could not create wasm_exec.js script element');
    }
    final completed = Completer<void>();
    script.setProperty('src'.toJS, asset.toJS);
    script.setProperty('onload'.toJS, (() {
      if (!completed.isCompleted) {
        completed.complete();
      }
    }).toJS);
    script.setProperty('onerror'.toJS, (() {
      if (!completed.isCompleted) {
        completed.completeError(StateError('failed to load $asset'));
      }
    }).toJS);
    final head = document.getProperty<JSObject>('head'.toJS);
    head.getProperty<JSFunction>('appendChild'.toJS).callAsFunction(head, script);
    await completed.future;
  }

  static Future<void> _waitForBridge(String libraryName) async {
    for (var attempt = 0; attempt < 200; attempt++) {
      final registry = globalContext.getProperty<JSObject?>('__flutterGoBridge'.toJS);
      if (registry?.getProperty<JSFunction?>(libraryName.toJS) != null) {
        return;
      }
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }
    throw StateError('Go Wasm bridge $libraryName did not register within 2 seconds');
  }
}
