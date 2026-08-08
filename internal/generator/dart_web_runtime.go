package generator

import (
	"fmt"
	"strconv"
	"strings"
)

func dartWebRuntimeSource(unit *unit) (string, error) {
	return dartWebRuntimeSourceFrom(unit, dartRuntimeSource)
}

func dartWebRuntimeSourceFrom(unit *unit, source string) (string, error) {
	common, err := dartCommonRuntimeSourceFrom(source)
	if err != nil {
		return "", err
	}
	web := dartWebPlatformRuntimeSource(unit)
	return strings.TrimSpace(common) + "\n\n" + strings.TrimSpace(web), nil
}

func dartNativeRuntimeSource(unit *unit) (string, error) {
	return dartNativeRuntimeSourceFrom(unit, dartRuntimeSource)
}

func dartNativeRuntimeSourceFrom(unit *unit, source string) (string, error) {
	common, platform, err := splitDartRuntimeSourceFrom(source)
	if err != nil {
		return "", err
	}
	platform = prepareDartPlatformRuntime(platform)
	platform = strings.ReplaceAll(platform, "__FGB_LIBRARY_NAME__", strconv.Quote(unit.LibraryName))
	platform = strings.ReplaceAll(platform, "__FGB_BRIDGE_CLASS__", unit.ClassName)
	platform += `

InternetAddress fgbInternalDecodeInternetAddress(String value) => InternetAddress(value);

String fgbInternalEncodeInternetAddress(InternetAddress value) => value.address;
`
	return strings.TrimSpace(common) + "\n\n" + strings.TrimSpace(platform), nil
}

func dartCommonRuntimeSource() (string, error) {
	return dartCommonRuntimeSourceFrom(dartRuntimeSource)
}

func dartCommonRuntimeSourceFrom(source string) (string, error) {
	common, _, err := splitDartRuntimeSourceFrom(source)
	return common, err
}

func dartNativePlatformRuntimeSource(unit *unit) (string, error) {
	return dartNativePlatformRuntimeSourceFrom(unit, dartRuntimeSource)
}

func dartNativePlatformRuntimeSourceFrom(unit *unit, source string) (string, error) {
	_, platform, err := splitDartRuntimeSourceFrom(source)
	if err != nil {
		return "", err
	}
	platform = prepareDartPlatformRuntime(platform)
	platform = strings.ReplaceAll(platform, "__FGB_LIBRARY_NAME__", strconv.Quote(unit.LibraryName))
	platform = strings.ReplaceAll(platform, "__FGB_BRIDGE_CLASS__", unit.ClassName)
	platform += `

InternetAddress fgbInternalDecodeInternetAddress(String value) => InternetAddress(value);

String fgbInternalEncodeInternetAddress(InternetAddress value) => value.address;
`
	return strings.TrimSpace(platform), nil
}

func dartWebPlatformRuntimeSource(unit *unit) string {
	web := prepareDartPlatformRuntime(dartWebBridgeSource)
	web = strings.ReplaceAll(web, "__FGB_BRIDGE_CLASS__", unit.ClassName)
	web = strings.ReplaceAll(web, "__FGB_LIBRARY_NAME__", strconv.Quote(unit.LibraryName))
	defaultInitializer := "throw UnsupportedError('FlutterGoBridge Web initialization requires a custom webInitializer in a pure-Dart package');"
	if unit.UseFlutterWebLoader {
		defaultInitializer = "WidgetsFlutterBinding.ensureInitialized();\n    await FgbWasmLoader.ensureReady();"
		web = dartFlutterWebLoaderSource(unit) + "\n\n" + web
	}
	web = strings.ReplaceAll(web, "__FGB_WEB_DEFAULT_INITIALIZER__", defaultInitializer)
	if unit.UsesInternetIP {
		web = `typedef InternetAddress = String;

InternetAddress fgbInternalDecodeInternetAddress(String value) => value;

String fgbInternalEncodeInternetAddress(InternetAddress value) => value;

` + web
	}
	return strings.TrimSpace(dartWebOpaqueSource) + "\n\n" + strings.TrimSpace(web)
}

func dartFlutterWebLoaderSource(unit *unit) string {
	source := strings.ReplaceAll(dartFlutterWebLoaderSourceTemplate, "__FGB_ASSET_PACKAGE__", strconv.Quote(unit.LibraryName))
	return strings.TrimSpace(source)
}

func splitDartRuntimeSourceFrom(source string) (common, native string, err error) {
	beforeOpaque, afterOpaque, found := strings.Cut(source, "abstract base class GoOpaque")
	if !found {
		return "", "", fmt.Errorf("Dart runtime GoOpaque marker is missing")
	}
	opaqueBody, afterAbsent, found := strings.Cut(afterOpaque, "abstract interface class GoAbsent")
	if !found {
		return "", "", fmt.Errorf("Dart runtime GoAbsent marker is missing")
	}
	commonTail, nativeTail, found := strings.Cut("abstract interface class GoAbsent"+afterAbsent, "final class _FgbData extends ffi.Struct")
	if !found {
		return "", "", fmt.Errorf("Dart runtime FFI marker is missing")
	}
	common = strings.TrimSpace(beforeOpaque) + "\n\n" + strings.TrimSpace(commonTail)
	common = strings.ReplaceAll(common, "_fgbGoErrorsFrom", "fgbInternalGoErrorsFrom")
	common = strings.ReplaceAll(common, "_FgbCallbackInvoker", "FgbInternalCallbackInvoker")
	common = strings.ReplaceAll(common, "_FgbStreamTarget", "FgbInternalStreamTarget")
	common += `

final _fgbInternalCodec = _FgbCodec();

Uint8List fgbInternalEncodeMethodCall(String method, List<Object?> arguments) =>
    _fgbInternalCodec.encodeMethodCall(method, arguments);

Object? fgbInternalDecodeEnvelope(Uint8List bytes) =>
    _fgbInternalCodec.decodeEnvelope(bytes);

Object? fgbInternalDecodeValue(Uint8List bytes) => _FgbReader(bytes).value();

Uint8List fgbInternalEncodeValue(Object? value) {
  final writer = _FgbWriter();
  _fgbInternalCodec._writeValue(writer, value);
  return writer.finish();
}
`
	native = strings.TrimSpace("abstract base class GoOpaque"+opaqueBody) + "\n\n" +
		strings.TrimSpace("final class _FgbData extends ffi.Struct"+nativeTail)
	return common, native, nil
}

func prepareDartPlatformRuntime(source string) string {
	source = strings.ReplaceAll(source, "_fgbGoErrorsFrom", "fgbInternalGoErrorsFrom")
	source = strings.ReplaceAll(source, "_FgbCallbackInvoker", "FgbInternalCallbackInvoker")
	source = strings.ReplaceAll(source, "_FgbStreamTarget", "FgbInternalStreamTarget")
	source = strings.ReplaceAll(source, "  static const _FgbCodec _codec = _FgbCodec();\n", "")
	source = strings.ReplaceAll(source, "_codec.encodeMethodCall", "fgbInternalEncodeMethodCall")
	source = strings.ReplaceAll(source, "_codec.decodeEnvelope", "fgbInternalDecodeEnvelope")
	source = strings.ReplaceAll(source, "_FgbReader(raw).value()", "fgbInternalDecodeValue(raw)")
	source = strings.ReplaceAll(source, "_FgbReader(request).value()", "fgbInternalDecodeValue(request)")
	source = strings.ReplaceAll(source, `  Uint8List _encodeCallbackReply(List<Object?> envelope) {
    final writer = _FgbWriter();
    _codec._writeValue(writer, envelope);
    return writer.finish();
  }`, `  Uint8List _encodeCallbackReply(List<Object?> envelope) {
    return fgbInternalEncodeValue(envelope);
  }`)
	return source
}

const dartWebOpaqueSource = `
/// Base class for generated opaque Go handles. Web uses a Dart Finalizer to
/// tell the Wasm runtime when the corresponding Go handle can be released.
abstract base class GoOpaque {
  GoOpaque({required this.fgbBridge, required this.fgbHandle}) {
    fgbBridge.fgbAttachOpaqueFinalizer(this, fgbHandle);
  }

  final __FGB_BRIDGE_CLASS__ fgbBridge;
  final int fgbHandle;
}
`

const dartFlutterWebLoaderSourceTemplate = `
const _fgbWasmAssetPackage = __FGB_ASSET_PACKAGE__;
const _fgbWasmAssetKeyRoot = 'packages/$_fgbWasmAssetPackage/assets/wasm';
const _fgbWasmAssetUrlRoot = 'assets/$_fgbWasmAssetKeyRoot';

final class FgbWasmManifest {
  FgbWasmManifest._(this.libraryName, this.wasmAsset, this.wasmExecAsset);

  final String libraryName;
  final String wasmAsset;
  final String wasmExecAsset;

  static FgbWasmManifest parse(String raw) {
    final value = jsonDecode(raw);
    if (value is! Map) {
      throw const FormatException(
        'fgb_wasm_manifest.json must contain an object',
      );
    }
    if (value['target'] != 'web-wasm' || value['schema_version'] != 1) {
      throw const FormatException('unsupported Gokit Web Wasm manifest');
    }
    final libraryName = value['library_name'];
    final artifacts = value['artifacts'];
    if (libraryName is! String || artifacts is! Map) {
      throw const FormatException(
        'Web Wasm manifest is missing library_name or artifacts',
      );
    }
    final wasm = artifacts.keys.whereType<String>().firstWhere(
      (key) => key.endsWith('.wasm'),
      orElse: () => throw const FormatException(
        'Web Wasm manifest has no .wasm artifact',
      ),
    );
    if (!artifacts.containsKey('wasm_exec.js')) {
      throw const FormatException(
        'Web Wasm manifest has no wasm_exec.js artifact',
      );
    }
    return FgbWasmManifest._(
      libraryName,
      '$_fgbWasmAssetUrlRoot/$wasm',
      '$_fgbWasmAssetUrlRoot/wasm_exec.js',
    );
  }
}

final class FgbWasmLoader {
  static Future<void>? _ready;

  static Future<void> ensureReady({AssetBundle? bundle}) async {
    if (_ready != null) {
      return _ready!;
    }
    _ready = _ensureReady(bundle: bundle);
    return _ready!;
  }

  static Future<void> _ensureReady({AssetBundle? bundle}) async {
    WidgetsFlutterBinding.ensureInitialized();
    final assets = bundle ?? rootBundle;
    final manifest = FgbWasmManifest.parse(
      await assets.loadString('$_fgbWasmAssetKeyRoot/fgb_wasm_manifest.json'),
    );
    await _loadScript(manifest.wasmExecAsset);
    final goConstructor = globalContext.getProperty<JSFunction?>('Go'.toJS);
    if (goConstructor == null) {
      throw StateError('wasm_exec.js did not register the Go constructor');
    }
    final go = goConstructor.callAsConstructor<JSObject>();
    final fetch = globalContext.getProperty<JSFunction>('fetch'.toJS);
    final response = fetch.callAsFunction(null, manifest.wasmAsset.toJS);
    final webAssembly = globalContext.getProperty<JSObject>('WebAssembly'.toJS);
    final instantiate = webAssembly.getProperty<JSFunction>(
      'instantiateStreaming'.toJS,
    );
    final importObject = go.getProperty<JSObject>('importObject'.toJS);
    final result =
        await (instantiate.callAsFunction(null, response, importObject)
                as JSPromise<JSAny?>)
            .toDart;
    if (result == null || !result.isA<JSObject>()) {
      throw StateError('WebAssembly.instantiateStreaming returned no instance');
    }
    final instance = (result as JSObject).getProperty<JSObject>(
      'instance'.toJS,
    );
    final run = go.getProperty<JSFunction>('run'.toJS);
    run.callAsFunction(go, instance);
    await _waitForBridge(manifest.libraryName);
  }

  static Future<void> _loadScript(String asset) async {
    final document = globalContext.getProperty<JSObject>('document'.toJS);
    final createElement = document.getProperty<JSFunction>(
      'createElement'.toJS,
    );
    final script = createElement.callAsFunction(document, 'script'.toJS);
    if (script == null || !script.isA<JSObject>()) {
      throw StateError('could not create wasm_exec.js script element');
    }
    final completed = Completer<void>();
    final scriptElement = script as JSObject;
    scriptElement.setProperty('src'.toJS, asset.toJS);
    scriptElement.setProperty(
      'onload'.toJS,
      (() {
        if (!completed.isCompleted) {
          completed.complete();
        }
      }).toJS,
    );
    scriptElement.setProperty(
      'onerror'.toJS,
      (() {
        if (!completed.isCompleted) {
          completed.completeError(StateError('failed to load $asset'));
        }
      }).toJS,
    );
    final head = document.getProperty<JSObject>('head'.toJS);
    head
        .getProperty<JSFunction>('appendChild'.toJS)
        .callAsFunction(head, scriptElement);
    await completed.future;
  }

  static Future<void> _waitForBridge(String libraryName) async {
    for (var attempt = 0; attempt < 200; attempt++) {
      final registry = globalContext.getProperty<JSObject?>(
        '__flutterGoBridge'.toJS,
      );
      if (registry?.getProperty<JSFunction?>(libraryName.toJS) != null) {
        return;
      }
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }
    throw StateError(
      'Go Wasm bridge $libraryName did not register within 2 seconds',
    );
  }
}
`

const dartWebBridgeSource = `
final class __FGB_BRIDGE_CLASS__ {
  __FGB_BRIDGE_CLASS__._() {
    _handleFinalizer = Finalizer<int>(_dropOpaque);
    _dartOpaqueReleaseFunction = ((JSNumber rawHandle) {
      _dartOpaqueObjects.remove(rawHandle.toDartInt);
    }).toJS;
    _callbackRequestFunction = ((JSUint8Array rawRequest) {
      return _handleCallbackRequest(rawRequest.toDart)
          .then((reply) => reply.toJS)
          .toJS;
    }).toJS;
    _streamEventFunction = ((JSUint8Array rawEvent) {
      return _handleStreamEvent(rawEvent.toDart).toJS;
    }).toJS;
    final attached = _webFunction('#attach').callAsFunction(
      null,
      _dartOpaqueReleaseFunction,
      _callbackRequestFunction,
      _streamEventFunction,
    );
    if (attached == null ||
        !attached.isA<JSBoolean>() ||
        !(attached as JSBoolean).toDart) {
      throw StateError('Go Wasm bridge rejected the Dart runtime attachment');
    }
  }

  static __FGB_BRIDGE_CLASS__? _instance;

  static __FGB_BRIDGE_CLASS__ get instance => _instance ??= __FGB_BRIDGE_CLASS__._();

  static __FGB_BRIDGE_CLASS__ open({String? libraryPath}) {
    if (libraryPath != null) {
      throw UnsupportedError('libraryPath is not used by the Web Wasm transport');
    }
    return instance;
  }

  static Future<void> initialize({
    String? libraryPath,
    Future<void> Function()? webInitializer,
  }) async {
    await (webInitializer ?? _defaultWebInitializer)();
    open(libraryPath: libraryPath);
  }

  static Future<void> _defaultWebInitializer() async {
    __FGB_WEB_DEFAULT_INITIALIZER__
  }

  static const _FgbCodec _codec = _FgbCodec();

  late final Finalizer<int> _handleFinalizer;
  late final JSFunction _dartOpaqueReleaseFunction;
  late final JSFunction _callbackRequestFunction;
  late final JSFunction _streamEventFunction;
  final Map<int, Object> _dartOpaqueObjects = <int, Object>{};
  final Map<int, FgbInternalStreamTarget> _streamTargets =
      <int, FgbInternalStreamTarget>{};
  final Map<Object, int> _streamHandles = <Object, int>{};
  int _dartOpaqueNextHandle = 0;
  int _streamNextHandle = 0;

  JSObject _webRegistry() {
    final registry = globalContext.getProperty<JSObject?>(
      '__flutterGoBridge'.toJS,
    );
    if (registry == null) {
      throw StateError(
        'Go Wasm runtime is not initialized; load wasm_exec.js and the .wasm artifact first',
      );
    }
    return registry;
  }

  JSFunction _webFunction(String suffix) {
    const libraryName = __FGB_LIBRARY_NAME__;
    final function = _webRegistry().getProperty<JSFunction?>(
      '$libraryName$suffix'.toJS,
    );
    if (function == null) {
      throw StateError('Go Wasm bridge function $libraryName$suffix is not registered');
    }
    return function;
  }

  Object? fgbInvokeSync(String method, List<Object?> arguments) {
    final request = _codec.encodeMethodCall(method, arguments);
    final rawResponse = _webFunction('').callAsFunction(null, request.toJS);
    if (rawResponse == null || !rawResponse.isA<JSUint8Array>()) {
      throw StateError('Go Wasm bridge returned a non-Uint8Array response');
    }
    return _codec.decodeEnvelope((rawResponse as JSUint8Array).toDart);
  }

  Future<Object?> fgbInvokeAsync(String method, List<Object?> arguments) async {
    final request = _codec.encodeMethodCall(method, arguments);
    final promise = _webFunction('#async').callAsFunction(null, request.toJS);
    if (promise == null || !promise.isA<JSPromise<JSAny?>>()) {
      throw StateError('Go Wasm bridge returned a non-Promise async response');
    }
    final rawResponse = await (promise as JSPromise<JSAny?>).toDart;
    if (rawResponse == null || !rawResponse.isA<JSUint8Array>()) {
      throw StateError('Go Wasm bridge returned a non-Uint8Array async response');
    }
    return _codec.decodeEnvelope((rawResponse as JSUint8Array).toDart);
  }

  void fgbAttachOpaqueFinalizer(Object object, int handle) {
    _handleFinalizer.attach(object, handle, detach: object);
  }

  void _dropOpaque(int handle) {
    _webFunction('#drop').callAsFunction(null, handle.toJS);
  }

  int fgbInternalRegisterDartOpaque(Object value) {
    final handle = ++_dartOpaqueNextHandle;
    _dartOpaqueObjects[handle] = value;
    return handle;
  }

  Object fgbInternalResolveDartOpaque(int handle, String path) {
    final value = _dartOpaqueObjects[handle];
    if (value == null) {
      throw StateError('$path: unknown or released DartOpaque handle $handle');
    }
    return value;
  }

  int fgbInternalRegisterCallback(Future<Object?> Function(List<Object?> args) invoker) {
    return fgbInternalRegisterDartOpaque(FgbInternalCallbackInvoker(invoker));
  }

  int fgbInternalRegisterStreamSink(
    StreamSink<dynamic> sink,
    void Function(Object? raw) add,
  ) {
    final existing = _streamHandles[sink];
    if (existing != null) {
      return existing;
    }
    final handle = ++_streamNextHandle;
    _streamTargets[handle] = FgbInternalStreamTarget(sink, add);
    _streamHandles[sink] = handle;
    sink.done.then(
      (_) => fgbInternalReleaseStreamSink(handle),
      onError: (Object _) => fgbInternalReleaseStreamSink(handle),
    );
    return handle;
  }

  void fgbInternalReleaseStreamSink(int handle, {bool notifyGo = true}) {
    final target = _streamTargets.remove(handle);
    if (target == null) {
      return;
    }
    _streamHandles.remove(target.sink);
    if (notifyGo) {
      _webFunction('#stream_cancel').callAsFunction(null, handle.toJS);
    }
  }

  void fgbInternalStartStream<T>(
    StreamController<T> controller,
    Future<void> call,
  ) {
    final handle = _streamHandles[controller.sink];
    controller.onCancel = () {
      if (handle != null) {
        fgbInternalReleaseStreamSink(handle);
      }
      scheduleMicrotask(() {
        if (!controller.isClosed) {
          controller.close();
        }
      });
    };
    call.then((_) {}, onError: (Object error, StackTrace stack) {
      if (handle != null) {
        fgbInternalReleaseStreamSink(handle);
      }
      if (!controller.isClosed) {
        controller.addError(error, stack);
        controller.close();
      }
    });
  }

  bool _handleStreamEvent(Uint8List raw) {
    final decoded = fgbInternalDecodeValue(raw);
    if (decoded is! List || decoded.length != 3) {
      return false;
    }
    final handle = decoded[0];
    final kind = decoded[1];
    if (handle is! int || kind is! int) {
      return false;
    }
    final target = _streamTargets[handle];
    if (target == null) {
      return false;
    }
    switch (kind) {
      case 0:
        target.add(decoded[2]);
        return true;
      case 1:
        fgbInternalReleaseStreamSink(handle, notifyGo: false);
        target.sink.close();
        return true;
      case 2:
        target.sink.addError(
          FgbPlatformException('stream_error', decoded[2] as String?, null),
        );
        return true;
      default:
        return false;
    }
  }

  Future<Uint8List> _handleCallbackRequest(Uint8List request) async {
    try {
      final decoded = fgbInternalDecodeValue(request);
      if (decoded is! List || decoded.length != 3) {
        throw const FormatException('malformed callback request');
      }
      final id = decoded[0];
      final handle = decoded[1];
      final arguments = decoded[2];
      if (id is! int || handle is! int || arguments is! List) {
        throw const FormatException('malformed callback request');
      }
      final entry = _dartOpaqueObjects[handle];
      if (entry is! FgbInternalCallbackInvoker) {
        throw StateError('unknown or released callback handle $handle');
      }
      final result = await entry.invoke(List<Object?>.of(arguments));
      return fgbInternalEncodeValue(<Object?>[0, result]);
    } catch (error) {
      return fgbInternalEncodeValue(
        <Object?>[1, 'callback_error', error.toString()],
      );
    }
  }
}
`
