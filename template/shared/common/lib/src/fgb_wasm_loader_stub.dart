/// Native builds do not need a Web Wasm loader.
final class FgbWasmManifest {}

final class FgbWasmLoader {
  static Future<void> ensureReady() async {}
}
