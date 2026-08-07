package generator

const dartRuntimeSource = `
final class FgbPlatformException implements Exception {
  FgbPlatformException(this.code, this.message, this.details, {this.goErrors});

  final String code;
  final String? message;
  final Object? details;

  /// The individual Go errors, when the call declares several error results.
  /// Null for a call with at most one error result - use [message] there.
  final FgbGoErrors? goErrors;

  @override
  String toString() => 'FgbPlatformException($code, $message, $details)';
}

/// The combined errors of a Go call that declares several error results: every
/// non-nil one is collected instead of only the first.
final class FgbGoErrors implements Exception {
  const FgbGoErrors(this.messages);

  final List<String> messages;

  int get length => messages.length;

  String operator [](int index) => messages[index];

  @override
  String toString() => 'FgbGoErrors(${messages.join('; ')})';
}

/// Reads the individual error messages out of an error envelope's details.
/// The standard codec sends the whole details map; the DCO path, which has no
/// map type, sends a plain list of messages.
FgbGoErrors? _fgbGoErrorsFrom(Object? details) {
  Object? errors = details;
  if (details is Map) {
    errors = details['errors'];
  }
  if (errors is! List) {
    return null;
  }
  return FgbGoErrors(
    List<String>.unmodifiable(errors.map((Object? error) => '$error')),
  );
}

/// Internal: unwraps the list carrying a call's several results.
List<Object?> fgbAsResultList(Object? value, int count) {
  if (value is! List || value.length != count) {
    throw FormatException('expected $count results, got $value');
  }
  return value;
}

/// Base class for generated opaque Go handles. The generated source files do
/// not need to expose dart:ffi; finalizer plumbing stays in this integration
/// library.
abstract base class GoOpaque implements ffi.Finalizable {
  GoOpaque({required this.fgbBridge, required this.fgbHandle}) {
    fgbBridge.fgbAttachOpaqueFinalizer(this, fgbHandle);
  }

  final __FGB_BRIDGE_CLASS__ fgbBridge;
  final int fgbHandle;
}

/// Marks a value that was nil on the Go side. Its members are not usable.
/// Check with an 'is GoAbsent' test when you need to tell an absent
/// interface value apart from a real one.
abstract interface class GoAbsent {}

/// Structural equality for the fields of a generated value class.
///
/// Dart compares List and Map by identity, which contradicts Go value
/// semantics: two structs decoded from the same wire bytes must be equal.
bool fgbInternalDeepEquals(Object? left, Object? right) {
  if (identical(left, right)) {
    return true;
  }
  if (left == null || right == null) {
    return false;
  }
  if (left is List && right is List) {
    if (left.length != right.length) {
      return false;
    }
    for (var index = 0; index < left.length; index++) {
      if (!fgbInternalDeepEquals(left[index], right[index])) {
        return false;
      }
    }
    return true;
  }
  if (left is Map && right is Map) {
    if (left.length != right.length) {
      return false;
    }
    // A scalar key can be looked up directly, which keeps this linear. A
    // generated class key also works, because its own == and hashCode are
    // structural. Anything else falls back to pairwise matching.
    if (left.keys.every(_fgbLooksUpByValue)) {
      for (final entry in left.entries) {
        if (!right.containsKey(entry.key)) {
          return false;
        }
        if (!fgbInternalDeepEquals(entry.value, right[entry.key])) {
          return false;
        }
      }
      return true;
    }
    final unmatched = right.entries.toList();
    for (final leftEntry in left.entries) {
      final index = unmatched.indexWhere(
        (rightEntry) =>
            fgbInternalDeepEquals(leftEntry.key, rightEntry.key) &&
            fgbInternalDeepEquals(leftEntry.value, rightEntry.value),
      );
      if (index < 0) {
        return false;
      }
      unmatched.removeAt(index);
    }
    return true;
  }
  return left == right;
}

bool _fgbLooksUpByValue(Object? key) =>
    key == null || key is String || key is num || key is bool || key is Enum;

/// Hash companion to [fgbInternalDeepEquals], combining with the same factor
/// the generated hashCode uses. Map entries are combined commutatively so
/// insertion order cannot change the result.
int fgbInternalDeepHash(Object? value) {
  if (value == null) {
    return 0;
  }
  if (value is List) {
    var result = 1;
    for (final item in value) {
      result = result * 31 + fgbInternalDeepHash(item);
    }
    return result;
  }
  if (value is Map) {
    var result = 0;
    for (final entry in value.entries) {
      result ^= fgbInternalDeepHash(entry.key) * 31 + fgbInternalDeepHash(entry.value);
    }
    return result;
  }
  return value.hashCode;
}

/// Wraps a registered Dart closure so the callback dispatcher can tell it
/// apart from plain DartOpaque objects sharing the same registry.
final class _FgbCallbackInvoker {
  _FgbCallbackInvoker(this.invoke);

  final Future<Object?> Function(List<Object?> args) invoke;
}

/// A Dart sink Go pushes values into, together with the typed add closure the
/// generated encoder built for it.
final class _FgbStreamTarget {
  _FgbStreamTarget(this.sink, this.add);

  final StreamSink<dynamic> sink;
  final void Function(Object? raw) add;
}

final class _FgbWriter {
  final BytesBuilder _bytes = BytesBuilder(copy: false);

  int get length => _bytes.length;

  void byte(int value) => _bytes.addByte(value & 0xff);

  void bytes(Iterable<int> value) =>
      _bytes.add(value is List<int> ? value : value.toList(growable: false));

  void alignment(int size) {
    final padding = (size - length % size) % size;
    if (padding != 0) {
      bytes(List<int>.filled(padding, 0));
    }
  }

  void size(int value) {
    if (value < 0) {
      throw FormatException('negative message size');
    }
    if (value < 254) {
      byte(value);
    } else if (value <= 0xffff) {
      byte(254);
      final data = ByteData(2)..setUint16(0, value, Endian.host);
      bytes(data.buffer.asUint8List());
    } else {
      byte(255);
      final data = ByteData(4)..setUint32(0, value, Endian.host);
      bytes(data.buffer.asUint8List());
    }
  }

  void int32(int value) {
    final data = ByteData(4)..setInt32(0, value, Endian.host);
    bytes(data.buffer.asUint8List());
  }

  void int64(int value) {
    final data = ByteData(8)..setInt64(0, value, Endian.host);
    bytes(data.buffer.asUint8List());
  }

  void float64(double value) {
    alignment(8);
    final data = ByteData(8)..setFloat64(0, value, Endian.host);
    bytes(data.buffer.asUint8List());
  }

	Uint8List finish() => _bytes.takeBytes();
}

final class _FgbReader {
  _FgbReader(this._bytes);

  final Uint8List _bytes;
  int _offset = 0;

  bool get hasRemaining => _offset < _bytes.length;
  int get remaining => _bytes.length - _offset;

  int byte() {
    if (_offset >= _bytes.length) {
      throw const FormatException('unexpected end of message');
    }
    return _bytes[_offset++];
  }

  Uint8List bytes(int length) {
    if (length < 0 || remaining < length) {
      throw const FormatException('message is truncated');
    }
    final result = Uint8List.sublistView(_bytes, _offset, _offset + length);
    _offset += length;
    return result;
  }

  void alignment(int size) {
    final padding = (size - _offset % size) % size;
    if (remaining < padding) {
      throw const FormatException('message alignment is truncated');
    }
    _offset += padding;
  }

  int size() {
    final marker = byte();
    if (marker < 254) {
      return marker;
    }
    if (marker == 254) {
      final data = ByteData.sublistView(bytes(2));
      return data.getUint16(0, Endian.host);
    }
    final data = ByteData.sublistView(bytes(4));
    return data.getUint32(0, Endian.host);
  }

  int count(int elementSize) {
    final length = size();
    if (length < 0 || (elementSize > 0 && length > remaining ~/ elementSize)) {
      throw const FormatException('message is truncated');
    }
    return length;
  }

  int int32() => ByteData.sublistView(bytes(4)).getInt32(0, Endian.host);

  int int64() => ByteData.sublistView(bytes(8)).getInt64(0, Endian.host);

  double float64() {
    alignment(8);
    return ByteData.sublistView(bytes(8)).getFloat64(0, Endian.host);
  }

  Object? value() {
    switch (byte()) {
      case 0:
        return null;
      case 1:
        return true;
      case 2:
        return false;
      case 3:
        return int32();
      case 4:
        return int64();
      case 5:
        // Flutter's base StandardMessageCodec exposes large integers received
        // from native code as their hexadecimal String representation.
        return utf8.decode(bytes(size()));
      case 6:
        return float64();
      case 7:
        return utf8.decode(bytes(size()));
      case 8:
        return bytes(size());
      case 9:
        final result = Int32List(count(4));
        alignment(4);
        for (var index = 0; index < result.length; index++) {
          result[index] = int32();
        }
        return result;
      case 10:
        final result = Int64List(count(8));
        alignment(8);
        for (var index = 0; index < result.length; index++) {
          result[index] = int64();
        }
        return result;
      case 11:
        final result = Float64List(count(8));
        alignment(8);
        for (var index = 0; index < result.length; index++) {
          result[index] = float64();
        }
        return result;
      case 12:
        final result = <Object?>[];
        for (var index = 0, length = count(1); index < length; index++) {
          result.add(value());
        }
        return result;
      case 13:
        final result = <Object?, Object?>{};
        for (var index = 0, length = count(2); index < length; index++) {
          result[value()] = value();
        }
        return result;
      case 14:
        final result = Float32List(count(4));
        alignment(4);
        for (var index = 0; index < result.length; index++) {
          final data = ByteData.sublistView(bytes(4));
          result[index] = data.getFloat32(0, Endian.host);
        }
        return result;
      default:
        throw const FormatException('unknown StandardMessageCodec type');
    }
  }
}

final class _FgbCodec {
  const _FgbCodec();

  Uint8List encodeMethodCall(String method, Object? arguments) {
    final writer = _FgbWriter();
    _writeValue(writer, method);
    _writeValue(writer, arguments);
    return writer.finish();
  }

  Object? decodeEnvelope(Uint8List bytes) {
    final reader = _FgbReader(bytes);
    final flag = reader.byte();
    if (flag == 0) {
      final result = reader.value();
      if (reader.hasRemaining) {
        throw const FormatException('result envelope has trailing bytes');
      }
      return result;
    }
    if (flag == 1) {
      final code = reader.value();
      final message = reader.value();
      final details = reader.value();
      if (reader.hasRemaining) {
        throw const FormatException('error envelope has trailing bytes');
      }
      if (code is! String) {
        throw const FormatException('error code is not a String');
      }
      if (message != null && message is! String) {
        throw const FormatException('error message is not a String or null');
      }
      return throw FgbPlatformException(
        code,
        message as String?,
        details,
        goErrors: _fgbGoErrorsFrom(details),
      );
    }
    throw const FormatException('unknown method envelope flag');
  }

  void _writeValue(_FgbWriter writer, Object? value) {
    if (value == null) {
      writer.byte(0);
    } else if (value is bool) {
      writer.byte(value ? 1 : 2);
    } else if (value is double) {
      writer.byte(6);
      writer.float64(value);
      // ignore: avoid_double_and_int_checks, JS uses the double branch above
    } else if (value is int) {
      if (value >= -0x80000000 && value <= 0x7fffffff) {
        writer.byte(3);
        writer.int32(value);
      } else {
        writer.byte(4);
        writer.int64(value);
      }
    } else if (value is BigInt) {
      // The base Dart codec does not emit tag 5. Use the same hexadecimal
      // string convention understood by the generated Go decoder.
      writer.byte(7);
      final raw = utf8.encode(value.toRadixString(16));
      writer.size(raw.length);
      writer.bytes(raw);
    } else if (value is String) {
      writer.byte(7);
      final raw = utf8.encode(value);
      writer.size(raw.length);
      writer.bytes(raw);
    } else if (value is Uint8List) {
      writer.byte(8);
      writer.size(value.length);
      writer.bytes(value);
    } else if (value is Int32List) {
      writer.byte(9);
      writer.size(value.length);
      writer.alignment(4);
      for (final item in value) {
        writer.int32(item);
      }
    } else if (value is Int64List) {
      writer.byte(10);
      writer.size(value.length);
      writer.alignment(8);
      for (final item in value) {
        writer.int64(item);
      }
    } else if (value is Float64List) {
      writer.byte(11);
      writer.size(value.length);
      writer.alignment(8);
      for (final item in value) {
        writer.float64(item);
      }
    } else if (value is Float32List) {
      writer.byte(14);
      writer.size(value.length);
      writer.alignment(4);
      for (final item in value) {
        final data = ByteData(4)..setFloat32(0, item, Endian.host);
        writer.bytes(data.buffer.asUint8List());
      }
    } else if (value is List) {
      writer.byte(12);
      writer.size(value.length);
      for (final item in value) {
        _writeValue(writer, item);
      }
    } else if (value is Map) {
      writer.byte(13);
      writer.size(value.length);
      value.forEach((key, item) {
        _writeValue(writer, key);
        _writeValue(writer, item);
      });
    } else {
      throw FormatException('type ${value.runtimeType} is not supported by StandardMessageCodec');
    }
  }
}

final class _FgbData extends ffi.Struct {
  external ffi.Pointer<ffi.Uint8> data;

  @ffi.Int64()
  external int len;
}

typedef _FgbInitNative = ffi.Int32 Function(ffi.Pointer<ffi.Void>);
typedef _FgbInitDart = int Function(ffi.Pointer<ffi.Void>);
typedef _FgbAllocNative = ffi.Pointer<ffi.Void> Function(ffi.Int64);
typedef _FgbAllocDart = ffi.Pointer<ffi.Void> Function(int);
typedef _FgbFreeNative = ffi.Void Function(ffi.Pointer<ffi.Void>);
typedef _FgbFreeDart = void Function(ffi.Pointer<ffi.Void>);
typedef _FgbSyncNative = _FgbData Function(ffi.Pointer<ffi.Void>, ffi.Int64);
typedef _FgbSyncDart = _FgbData Function(ffi.Pointer<ffi.Void>, int);
typedef _FgbAsyncNative = ffi.Void Function(ffi.Pointer<ffi.Void>, ffi.Int64, ffi.Int64);
typedef _FgbAsyncDart = void Function(ffi.Pointer<ffi.Void>, int, int);
typedef _FgbDropNative = ffi.Void Function(ffi.Pointer<ffi.Void>);
typedef _FgbDropDart = void Function(ffi.Pointer<ffi.Void>);
typedef _FgbCstNative = ffi.Pointer<ffi.Void> Function(ffi.Int32, ffi.Pointer<ffi.Void>);
typedef _FgbCstDart = ffi.Pointer<ffi.Void> Function(int, ffi.Pointer<ffi.Void>);
typedef _FgbCstAsyncNative = ffi.Void Function(ffi.Int32, ffi.Pointer<ffi.Void>, ffi.Int64);
typedef _FgbCstAsyncDart = void Function(int, ffi.Pointer<ffi.Void>, int);
typedef _FgbDcoFreeNative = ffi.Void Function(ffi.Pointer<ffi.Void>);
typedef _FgbDcoFreeDart = void Function(ffi.Pointer<ffi.Void>);
typedef _FgbIsolateAttachNative = ffi.Void Function(ffi.Int64, ffi.Int64, ffi.Int64);
typedef _FgbIsolateAttachDart = void Function(int, int, int);
typedef _FgbCallbackResultNative = ffi.Void Function(ffi.Int64, ffi.Pointer<ffi.Void>, ffi.Int64);
typedef _FgbCallbackResultDart = void Function(int, ffi.Pointer<ffi.Void>, int);
typedef _FgbStreamCancelNative = ffi.Void Function(ffi.Int64);
typedef _FgbStreamCancelDart = void Function(int);

final class _FgbBindings {
  _FgbBindings(this.library)
      : init = library.lookupFunction<_FgbInitNative, _FgbInitDart>('fgb_init'),
        alloc = library.lookupFunction<_FgbAllocNative, _FgbAllocDart>('fgb_alloc'),
        free = library.lookupFunction<_FgbFreeNative, _FgbFreeDart>('fgb_free'),
        call = library.lookupFunction<_FgbSyncNative, _FgbSyncDart>('fgb'),
        callAsync = library.lookupFunction<_FgbAsyncNative, _FgbAsyncDart>('fgb_async'),
        drop = library.lookupFunction<_FgbDropNative, _FgbDropDart>('fgb_drop'),
        cst = library.lookupFunction<_FgbCstNative, _FgbCstDart>('fgb_cst'),
        cstAsync = library.lookupFunction<_FgbCstAsyncNative, _FgbCstAsyncDart>('fgb_cst_async'),
        dcoFree = library.lookupFunction<_FgbDcoFreeNative, _FgbDcoFreeDart>('fgb_dco_free'),
        isolateAttach = library.lookupFunction<_FgbIsolateAttachNative, _FgbIsolateAttachDart>('fgb_isolate_attach'),
        callbackResult = library.lookupFunction<_FgbCallbackResultNative, _FgbCallbackResultDart>('fgb_callback_result'),
        streamCancel = library.lookupFunction<_FgbStreamCancelNative, _FgbStreamCancelDart>('fgb_stream_cancel'),
        dropAddress = library.lookup<ffi.NativeFunction<_FgbDropNative>>('fgb_drop');

  final ffi.DynamicLibrary library;
  final int Function(ffi.Pointer<ffi.Void>) init;
  final ffi.Pointer<ffi.Void> Function(int) alloc;
  final void Function(ffi.Pointer<ffi.Void>) free;
  final _FgbData Function(ffi.Pointer<ffi.Void>, int) call;
  final void Function(ffi.Pointer<ffi.Void>, int, int) callAsync;
  final void Function(ffi.Pointer<ffi.Void>) drop;
  final ffi.Pointer<ffi.Void> Function(int, ffi.Pointer<ffi.Void>) cst;
  final void Function(int, ffi.Pointer<ffi.Void>, int) cstAsync;
  final void Function(ffi.Pointer<ffi.Void>) dcoFree;
  final void Function(int, int, int) isolateAttach;
  final void Function(int, ffi.Pointer<ffi.Void>, int) callbackResult;
  final void Function(int) streamCancel;
  final ffi.Pointer<ffi.NativeFunction<_FgbDropNative>> dropAddress;
}

final class _FgbDcoTypedData extends ffi.Struct {
  @ffi.Uint32()
  external int type;

  @ffi.IntPtr()
  external int length;

  external ffi.Pointer<ffi.Uint8> values;
}

final class _FgbDcoArray extends ffi.Struct {
  @ffi.IntPtr()
  external int length;

  external ffi.Pointer<ffi.Pointer<_FgbDcoObject>> values;
}

typedef _FgbDcoFinalizerNative = ffi.Void Function(ffi.Pointer<ffi.Void>, ffi.Pointer<ffi.Void>);

final class _FgbDcoExternalTypedData extends ffi.Struct {
  @ffi.Uint32()
  external int type;

  @ffi.IntPtr()
  external int length;

  external ffi.Pointer<ffi.Uint8> data;
  external ffi.Pointer<ffi.Void> peer;
  external ffi.Pointer<ffi.NativeFunction<_FgbDcoFinalizerNative>> callback;
}

final class _FgbDcoNativePointer extends ffi.Struct {
  @ffi.IntPtr()
  external int ptr;

  @ffi.IntPtr()
  external int size;

  external ffi.Pointer<ffi.NativeFunction<_FgbDcoFinalizerNative>> callback;
}

final class _FgbDcoValue extends ffi.Union {
  @ffi.Bool()
  external bool asBool;

  @ffi.Int32()
  external int asInt32;

  @ffi.Int64()
  external int asInt64;

  @ffi.Double()
  external double asDouble;

  external ffi.Pointer<ffi.Char> asString;

  external _FgbDcoArray asArray;

  external _FgbDcoTypedData asTypedData;

  external _FgbDcoExternalTypedData asExternalTypedData;

  external _FgbDcoNativePointer asNativePointer;
}

final class _FgbDcoObject extends ffi.Struct {
  @ffi.Uint32()
  external int type;

  external _FgbDcoValue value;
}

final class _FgbArena {
  _FgbArena(this.bridge);

  final __FGB_BRIDGE_CLASS__ bridge;
  final List<ffi.Pointer<ffi.Void>> _allocations = <ffi.Pointer<ffi.Void>>[];

  ffi.Pointer<T> allocate<T extends ffi.NativeType>(int bytes) {
    final pointer = bridge._bindings.alloc(bytes);
    if (pointer == ffi.nullptr) {
      throw StateError('fgb_alloc returned null');
    }
    if (bytes > 0) {
      pointer.cast<ffi.Uint8>().asTypedList(bytes).fillRange(0, bytes, 0);
    }
    _allocations.add(pointer);
    return pointer.cast<T>();
  }

  ffi.Pointer<ffi.Uint8> bytes(List<int> value) {
    final pointer = allocate<ffi.Uint8>(value.isEmpty ? 1 : value.length);
    if (value.isNotEmpty) {
      pointer.asTypedList(value.length).setAll(0, value);
    }
    return pointer;
  }

  void close() {
    for (final pointer in _allocations.reversed) {
      bridge._bindings.free(pointer);
    }
    _allocations.clear();
  }
}

final class __FGB_BRIDGE_CLASS__ {
  __FGB_BRIDGE_CLASS__._(this._bindings, this._libraryPath)
      : _handleFinalizer = ffi.NativeFinalizer(_bindings.dropAddress) {
    final status = _bindings.init(ffi.NativeApi.initializeApiDLData);
    if (status != 0) {
      throw StateError('Dart API DL initialization failed (status $status)');
    }
    // Go notifies this port when the last Go copy of a DartOpaque value was
    // collected; the entry keeping the Dart object alive is then dropped. The
    // port must not keep the isolate alive on its own.
    _dartOpaqueReleases.keepIsolateAlive = false;
    _dartOpaqueReleases.handler = (Object? message) {
      if (message is int) {
        _dartOpaqueObjects.remove(message);
      }
    };
    // Go posts callback invocation requests here whenever an //fgb:async call
    // invokes a Dart-supplied closure; the goroutine parks until the reply is
    // delivered through fgb_callback_result.
    _callbackRequests.keepIsolateAlive = false;
    _callbackRequests.handler = _handleCallbackRequest;
    // Go posts stream items, errors and completion for every registered
    // StreamSink here.
    _streamEvents.keepIsolateAlive = false;
    _streamEvents.handler = _handleStreamEvent;
    _bindings.isolateAttach(
      _dartOpaqueReleases.sendPort.nativePort,
      _callbackRequests.sendPort.nativePort,
      _streamEvents.sendPort.nativePort,
    );
  }

  static __FGB_BRIDGE_CLASS__? _instance;

  static __FGB_BRIDGE_CLASS__ get instance => _instance ??= __FGB_BRIDGE_CLASS__._open();

  static __FGB_BRIDGE_CLASS__ open({String? libraryPath}) {
    final existing = _instance;
    if (existing != null) {
      if (libraryPath != null && existing._libraryPath != libraryPath) {
        throw StateError('FlutterGoBridge is already initialized with another library');
      }
      return existing;
    }
    return _instance = __FGB_BRIDGE_CLASS__._open(libraryPath: libraryPath);
  }

  static void initialize({String? libraryPath}) => open(libraryPath: libraryPath);

  static __FGB_BRIDGE_CLASS__ _open({String? libraryPath}) {
    final library = libraryPath == null
        ? _openDefaultLibrary()
        : ffi.DynamicLibrary.open(libraryPath);
    return __FGB_BRIDGE_CLASS__._(_FgbBindings(library), libraryPath);
  }

  static ffi.DynamicLibrary _openDefaultLibrary() {
    if (Platform.isMacOS || Platform.isIOS) {
      return ffi.DynamicLibrary.process();
    }
    const libraryName = __FGB_LIBRARY_NAME__;
    if (Platform.isWindows) {
      return ffi.DynamicLibrary.open('$libraryName.dll');
    }
    return ffi.DynamicLibrary.open('lib$libraryName.so');
  }

  final _FgbBindings _bindings;
  final String? _libraryPath;
  final ffi.NativeFinalizer _handleFinalizer;
  final RawReceivePort _dartOpaqueReleases = RawReceivePort();
  final RawReceivePort _callbackRequests = RawReceivePort();
  final RawReceivePort _streamEvents = RawReceivePort();
  final Map<int, Object> _dartOpaqueObjects = <int, Object>{};
  final Map<int, _FgbStreamTarget> _streamTargets = <int, _FgbStreamTarget>{};
  final Map<Object, int> _streamHandles = <Object, int>{};
  int _dartOpaqueNextHandle = 0;
  int _streamNextHandle = 0;
  static const _FgbCodec _codec = _FgbCodec();

  /// Internal: registers a Dart sink Go may push values into, and returns the
  /// handle Go uses to address it. Registering the same sink twice reuses the
  /// handle, so a call may pass one sink through several parameters.
  int fgbInternalRegisterStreamSink(StreamSink<dynamic> sink, void Function(Object? raw) add) {
    final existing = _streamHandles[sink];
    if (existing != null) {
      return existing;
    }
    final handle = ++_streamNextHandle;
    _streamTargets[handle] = _FgbStreamTarget(sink, add);
    _streamHandles[sink] = handle;
    // Closing the sink (the owner disposing its StreamController) retires the
    // registration and tells Go to stop producing.
    sink.done.then(
      (_) => fgbInternalReleaseStreamSink(handle),
      onError: (Object _) => fgbInternalReleaseStreamSink(handle),
    );
    return handle;
  }

  /// Internal: retires a stream registration and notifies Go, which then
  /// reports fgb.ErrStreamClosed to whoever keeps adding values.
  void fgbInternalReleaseStreamSink(int handle, {bool notifyGo = true}) {
    final target = _streamTargets.remove(handle);
    if (target == null) {
      return;
    }
    _streamHandles.remove(target.sink);
    if (notifyGo) {
      _bindings.streamCancel(handle);
    }
  }

  /// Internal: wires a call that owns its stream. Cancelling the subscription
  /// releases the sink and retires the controller; a failing call surfaces as
  /// a stream error.
  void fgbInternalStartStream<T>(StreamController<T> controller, Future<void> call) {
    final handle = _streamHandles[controller.sink];
    controller.onCancel = () {
      if (handle != null) {
        fgbInternalReleaseStreamSink(handle);
      }
      // The subscription is gone, so nothing will ever read from this
      // controller again: close it so its "done" future completes instead of
      // leaving an open controller behind. Closing from inside onCancel is
      // deferred to a microtask - returning close()'s future here would wait
      // on the very cancellation that is still in progress.
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

  void _handleStreamEvent(Object? message) {
    Uint8List? raw;
    if (message is Uint8List) {
      raw = message;
    } else if (message is List<int>) {
      raw = Uint8List.fromList(message);
    }
    if (raw == null) {
      return;
    }
    final decoded = _FgbReader(raw).value();
    if (decoded is! List || decoded.length != 3) {
      return;
    }
    final handle = decoded[0];
    final kind = decoded[1];
    if (handle is! int || kind is! int) {
      return;
    }
    final target = _streamTargets[handle];
    if (target == null) {
      return;
    }
    switch (kind) {
      case 0:
        target.add(decoded[2]);
        break;
      case 1:
        fgbInternalReleaseStreamSink(handle, notifyGo: false);
        target.sink.close();
        break;
      case 2:
        target.sink.addError(
          FgbPlatformException('stream_error', decoded[2] as String?, null),
        );
        break;
    }
  }

  /// Internal: registers a Dart object crossing into Go as a DartOpaque
  /// handle. The registry entry keeps the object alive while Go holds it.
  int fgbInternalRegisterDartOpaque(Object value) {
    final handle = ++_dartOpaqueNextHandle;
    _dartOpaqueObjects[handle] = value;
    return handle;
  }

  /// Internal: resolves a DartOpaque handle returned by Go.
  Object fgbInternalResolveDartOpaque(int handle, String path) {
    final value = _dartOpaqueObjects[handle];
    if (value == null) {
      throw StateError('$path: unknown or released DartOpaque handle $handle');
    }
    return value;
  }

  /// Internal: registers a Dart closure for invocation from Go. The stored
  /// invoker decodes the wire arguments, runs the user closure - awaiting it
  /// when the closure is async - and returns the wire-encoded result. It
  /// shares the DartOpaque registry, so Go dropping its last reference
  /// releases the closure.
  int fgbInternalRegisterCallback(Future<Object?> Function(List<Object?> args) invoker) {
    return fgbInternalRegisterDartOpaque(_FgbCallbackInvoker(invoker));
  }

  void _handleCallbackRequest(Object? message) {
    Uint8List? request;
    if (message is Uint8List) {
      request = message;
    } else if (message is List<int>) {
      request = Uint8List.fromList(message);
    }
    if (request == null) {
      return;
    }
    final decoded = _FgbReader(request).value();
    if (decoded is! List || decoded.length != 3) {
      return;
    }
    final id = decoded[0];
    final handle = decoded[1];
    final arguments = decoded[2];
    if (id is! int) {
      return;
    }
    if (handle is! int || arguments is! List) {
      _deliverCallbackReply(id)(
        _encodeCallbackReply(<Object?>[1, 'callback_error', 'malformed callback request']),
      );
      return;
    }
    Future<Uint8List>(() async {
      final entry = _dartOpaqueObjects[handle];
      if (entry is! _FgbCallbackInvoker) {
        throw StateError('unknown or released callback handle $handle');
      }
      final result = await entry.invoke(List<Object?>.of(arguments));
      return _encodeCallbackReply(<Object?>[0, result]);
    }).catchError((Object error) {
      return _encodeCallbackReply(<Object?>[1, 'callback_error', error.toString()]);
    }).then(_deliverCallbackReply(id));
  }

  Uint8List _encodeCallbackReply(List<Object?> envelope) {
    final writer = _FgbWriter();
    _codec._writeValue(writer, envelope);
    return writer.finish();
  }

  void Function(Uint8List) _deliverCallbackReply(int id) {
    return (Uint8List reply) {
      final pointer = _bindings.alloc(reply.length);
      if (pointer == ffi.nullptr) {
        _bindings.callbackResult(id, ffi.nullptr, 0);
        return;
      }
      try {
        pointer.cast<ffi.Uint8>().asTypedList(reply.length).setAll(0, reply);
        _bindings.callbackResult(id, pointer, reply.length);
      } finally {
        _bindings.free(pointer);
      }
    };
  }

  /// Internal entrypoint used by generated per-source Dart API files.
  Object? fgbInvokeSync(String method, List<Object?> arguments) {
    final request = _codec.encodeMethodCall(method, arguments);
    return _codec.decodeEnvelope(_syncBytes(request));
  }

  /// Internal entrypoint used by generated per-source Dart API files.
  Future<Object?> fgbInvokeAsync(String method, List<Object?> arguments) async {
    final request = _codec.encodeMethodCall(method, arguments);
    return _codec.decodeEnvelope(await _asyncBytes(request));
  }

  Object? fgbInvokeCstSync(int method, ffi.Pointer<ffi.Void> args) {
    final pointer = _bindings.cst(method, args);
    if (pointer == ffi.nullptr) {
      throw StateError('fgb_cst returned null');
    }
    try {
      return _decodeDcoEnvelope(_decodeDco(pointer.cast<_FgbDcoObject>().ref));
    } finally {
      _bindings.dcoFree(pointer);
    }
  }

  Future<Object?> fgbInvokeCstAsync(int method, ffi.Pointer<ffi.Void> args) async {
    final port = ReceivePort();
    try {
      _bindings.cstAsync(method, args, port.sendPort.nativePort);
      return _decodeDcoEnvelope(await port.first);
    } finally {
      port.close();
    }
  }

  Object? _decodeDco(_FgbDcoObject object) {
    switch (object.type) {
      case 0:
        return null;
      case 1:
        return object.value.asBool;
      case 2:
        return object.value.asInt32;
      case 3:
        return object.value.asInt64;
      case 4:
        return object.value.asDouble;
      case 5:
        final pointer = object.value.asString.cast<ffi.Uint8>();
        var length = 0;
        while ((pointer + length).value != 0) {
          length++;
        }
        return utf8.decode(pointer.asTypedList(length));
      case 6:
        final array = object.value.asArray;
        return List<Object?>.generate(
          array.length,
          (index) => _decodeDco((array.values + index).value.ref),
          growable: false,
        );
      case 7:
        final typed = object.value.asTypedData;
        final length = typed.length;
        switch (typed.type) {
          case 2:
            return Uint8List.fromList(typed.values.asTypedList(length));
          case 6:
            return Int32List.fromList(typed.values.cast<ffi.Int32>().asTypedList(length));
          case 8:
            return Int64List.fromList(typed.values.cast<ffi.Int64>().asTypedList(length));
          case 11:
            return Float64List.fromList(typed.values.cast<ffi.Double>().asTypedList(length));
          default:
            throw FormatException('unsupported Dart_CObject typed data ${typed.type}');
        }
      default:
        throw FormatException('unsupported Dart_CObject type ${object.type}');
    }
  }

  Object? _decodeDcoEnvelope(Object? raw) {
    if (raw == -1) {
      throw StateError('the native side could not allocate the reply');
    }
    if (raw is! List || raw.length < 2 || raw.first is! int) {
      throw const FormatException('invalid DCO envelope');
    }
    if (raw.first == 0) {
      return raw[1];
    }
    if (raw.first == 1 && raw.length >= 4) {
      throw FgbPlatformException(
        raw[1] as String,
        raw[2] as String?,
        raw[3],
        goErrors: _fgbGoErrorsFrom(raw[3]),
      );
    }
    throw const FormatException('unknown DCO envelope flag');
  }

  /// Attaches framework-owned cleanup for an opaque Go handle.
  void fgbAttachOpaqueFinalizer(ffi.Finalizable object, int handle) {
    _handleFinalizer.attach(
      object,
      ffi.Pointer<ffi.Void>.fromAddress(handle),
      detach: object,
    );
  }

  Uint8List _syncBytes(Uint8List request) {
    final pointer = _bindings.alloc(request.length);
    if (pointer == ffi.nullptr) {
      throw StateError('fgb_alloc returned null');
    }
    try {
      pointer.cast<ffi.Uint8>().asTypedList(request.length).setAll(0, request);
      final result = _bindings.call(pointer, request.length);
      if (result.data == ffi.nullptr && result.len != 0) {
        throw StateError('fgb returned an invalid buffer');
      }
      try {
        if (result.len == 0) {
          return Uint8List(0);
        }
        return Uint8List.fromList(result.data.asTypedList(result.len));
      } finally {
        if (result.data != ffi.nullptr) {
          _bindings.free(result.data.cast());
        }
      }
    } finally {
      _bindings.free(pointer);
    }
  }

  Future<Uint8List> _asyncBytes(Uint8List request) async {
    final port = ReceivePort();
    final pointer = _bindings.alloc(request.length);
    if (pointer == ffi.nullptr) {
      port.close();
      throw StateError('fgb_alloc returned null');
    }
    try {
      try {
        pointer.cast<ffi.Uint8>().asTypedList(request.length).setAll(0, request);
        _bindings.callAsync(pointer, request.length, port.sendPort.nativePort);
      } finally {
        _bindings.free(pointer);
      }
      final message = await port.first;
      if (message is Uint8List) {
        return message;
      }
      if (message is List<int>) {
        return Uint8List.fromList(message);
      }
      throw StateError('fgb_async returned an invalid message');
    } finally {
      port.close();
    }
  }
}
`
