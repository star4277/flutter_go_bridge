package generator

import (
	"fmt"
	"strconv"
	"strings"
)

func dartWebRuntimeSource(unit *unit) (string, error) {
	beforeOpaque, afterOpaque, found := strings.Cut(dartRuntimeSource, "abstract base class GoOpaque")
	if !found {
		return "", fmt.Errorf("Web Dart runtime GoOpaque marker is missing")
	}
	_, afterOpaque, found = strings.Cut(afterOpaque, "abstract interface class GoAbsent")
	if !found {
		return "", fmt.Errorf("Web Dart runtime GoAbsent marker is missing")
	}
	commonTail, _, found := strings.Cut("abstract interface class GoAbsent"+afterOpaque, "final class _FgbData extends ffi.Struct")
	if !found {
		return "", fmt.Errorf("Web Dart runtime FFI marker is missing")
	}

	web := strings.ReplaceAll(dartWebBridgeSource, "__FGB_BRIDGE_CLASS__", unit.ClassName)
	web = strings.ReplaceAll(web, "__FGB_LIBRARY_NAME__", strconv.Quote(unit.LibraryName))
	if unit.UsesInternetIP {
		web = "typedef InternetAddress = String;\n\n" + web
	}
	return strings.TrimSpace(beforeOpaque) + "\n\n" + strings.TrimSpace(dartWebOpaqueSource) +
		"\n\n" + strings.TrimSpace(commonTail) + "\n\n" + strings.TrimSpace(web), nil
}

const dartWebOpaqueSource = `
/// Base class for generated opaque Go handles. Web methods using opaque
/// handles currently fail before transport, so no native finalizer is needed.
abstract base class GoOpaque {
  GoOpaque({required this.fgbBridge, required this.fgbHandle});

  final __FGB_BRIDGE_CLASS__ fgbBridge;
  final int fgbHandle;
}
`

const dartWebBridgeSource = `
final class __FGB_BRIDGE_CLASS__ {
  __FGB_BRIDGE_CLASS__._();

  static __FGB_BRIDGE_CLASS__? _instance;

  static __FGB_BRIDGE_CLASS__ get instance => _instance ??= __FGB_BRIDGE_CLASS__._();

  static __FGB_BRIDGE_CLASS__ open({String? libraryPath}) {
    if (libraryPath != null) {
      throw UnsupportedError('libraryPath is not used by the Web Wasm transport');
    }
    return instance;
  }

  static void initialize({String? libraryPath}) => open(libraryPath: libraryPath);

  static const _FgbCodec _codec = _FgbCodec();

  Object? fgbInvokeSync(String method, List<Object?> arguments) {
    const libraryName = __FGB_LIBRARY_NAME__;
    final registry = globalContext.getProperty<JSObject?>('__flutterGoBridge'.toJS);
    if (registry == null) {
      throw StateError('Go Wasm runtime is not initialized; load wasm_exec.js and the .wasm artifact first');
    }
    final invoke = registry.getProperty<JSFunction?>(libraryName.toJS);
    if (invoke == null) {
      throw StateError('Go Wasm bridge $libraryName is not registered');
    }
    final request = _codec.encodeMethodCall(method, arguments);
    final rawResponse = invoke.callAsFunction(null, request.toJS);
    if (rawResponse is! JSUint8Array) {
      throw StateError('Go Wasm bridge returned a non-Uint8Array response');
    }
    return _codec.decodeEnvelope(rawResponse.toDart);
  }

  Future<Object?> fgbInvokeAsync(String method, List<Object?> arguments) async {
    return fgbInvokeSync(method, arguments);
  }

  void fgbAttachOpaqueFinalizer(Object object, int handle) {}

  int fgbInternalRegisterDartOpaque(Object value) {
    throw UnsupportedError('DartOpaque is not supported by the Web Wasm transport');
  }

  Object fgbInternalResolveDartOpaque(int handle, String path) {
    throw UnsupportedError('$path: DartOpaque is not supported by the Web Wasm transport');
  }

  int fgbInternalRegisterCallback(Future<Object?> Function(List<Object?> args) invoker) {
    throw UnsupportedError('Dart callbacks are not supported by the Web Wasm transport');
  }

  int fgbInternalRegisterStreamSink(
    StreamSink<dynamic> sink,
    void Function(Object? raw) add,
  ) {
    throw UnsupportedError('streams are not supported by the Web Wasm transport');
  }

  void fgbInternalStartStream<T>(StreamController<T> controller, Future<void> call) {
    throw UnsupportedError('streams are not supported by the Web Wasm transport');
  }
}
`
