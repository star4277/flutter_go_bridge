// Drives a generated Go Wasm bridge the way the generated Dart Web wire does:
// the standard method codec over Uint8Array, plus the attach/callback/stream/
// drop entrypoints the bridge registers on globalThis.__flutterGoBridge.
//
// Usage: node web_wasm_harness.mjs <build-dir> <library-name>
// The build dir must contain bridge.wasm and wasm_exec.js.
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';

const [buildDir, LIBRARY] = process.argv.slice(2);
if (!buildDir || !LIBRARY) {
  throw new Error('usage: node web_wasm_harness.mjs <build-dir> <library-name>');
}

const NULL = 0, TRUE = 1, FALSE = 2, INT32 = 3, INT64 = 4, BIGINT = 5,
      FLOAT64 = 6, STRING = 7, UINT8LIST = 8, LIST = 12, MAP = 13;

class Writer {
  constructor() {
    this.bytes = [];
  }
  u8(value) {
    this.bytes.push(value & 0xff);
  }
  raw(values) {
    for (const value of values) {
      this.bytes.push(value & 0xff);
    }
  }
  size(value) {
    if (value < 254) {
      this.u8(value);
    } else if (value <= 0xffff) {
      this.u8(254);
      this.raw(new Uint8Array(Uint16Array.of(value).buffer));
    } else {
      this.u8(255);
      this.raw(new Uint8Array(Uint32Array.of(value).buffer));
    }
  }
  align(alignment) {
    const padding = (alignment - (this.bytes.length % alignment)) % alignment;
    for (let i = 0; i < padding; i++) {
      this.u8(0);
    }
  }
  string(value) {
    const encoded = new TextEncoder().encode(value);
    this.size(encoded.length);
    this.raw(encoded);
  }
  value(value) {
    if (value === null || value === undefined) {
      this.u8(NULL);
    } else if (value === true) {
      this.u8(TRUE);
    } else if (value === false) {
      this.u8(FALSE);
    } else if (typeof value === 'bigint') {
      this.u8(INT64);
      this.raw(new Uint8Array(BigInt64Array.of(value).buffer));
    } else if (typeof value === 'number') {
      if (Number.isInteger(value) && value >= -2147483648 && value <= 2147483647) {
        this.u8(INT32);
        this.raw(new Uint8Array(Int32Array.of(value).buffer));
      } else {
        this.u8(FLOAT64);
        this.align(8);
        this.raw(new Uint8Array(Float64Array.of(value).buffer));
      }
    } else if (typeof value === 'string') {
      this.u8(STRING);
      this.string(value);
    } else if (value instanceof Uint8Array) {
      this.u8(UINT8LIST);
      this.size(value.length);
      this.raw(value);
    } else if (Array.isArray(value)) {
      this.u8(LIST);
      this.size(value.length);
      for (const item of value) {
        this.value(item);
      }
    } else if (value instanceof Map) {
      this.u8(MAP);
      this.size(value.size);
      for (const [key, item] of value) {
        this.value(key);
        this.value(item);
      }
    } else {
      throw new Error(`cannot encode ${typeof value}: ${value}`);
    }
  }
  finish() {
    return Uint8Array.from(this.bytes);
  }
}

class Reader {
  constructor(bytes) {
    this.bytes = bytes;
    this.view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    this.offset = 0;
  }
  u8() {
    return this.view.getUint8(this.offset++);
  }
  size() {
    const first = this.u8();
    if (first < 254) {
      return first;
    }
    if (first === 254) {
      const value = this.view.getUint16(this.offset, true);
      this.offset += 2;
      return value;
    }
    const value = this.view.getUint32(this.offset, true);
    this.offset += 4;
    return value;
  }
  align(alignment) {
    this.offset += (alignment - (this.offset % alignment)) % alignment;
  }
  bytesOf(length) {
    const slice = this.bytes.subarray(this.offset, this.offset + length);
    this.offset += length;
    return slice;
  }
  value() {
    const type = this.u8();
    switch (type) {
      case NULL: return null;
      case TRUE: return true;
      case FALSE: return false;
      case INT32: {
        const value = this.view.getInt32(this.offset, true);
        this.offset += 4;
        return value;
      }
      case INT64: {
        const value = this.view.getBigInt64(this.offset, true);
        this.offset += 8;
        return Number(value);
      }
      case BIGINT: return BigInt(`0x${new TextDecoder().decode(this.bytesOf(this.size()))}`);
      case FLOAT64: {
        this.align(8);
        const value = this.view.getFloat64(this.offset, true);
        this.offset += 8;
        return value;
      }
      case STRING: return new TextDecoder().decode(this.bytesOf(this.size()));
      case UINT8LIST: return Uint8Array.from(this.bytesOf(this.size()));
      case LIST: {
        const count = this.size();
        const result = [];
        for (let i = 0; i < count; i++) {
          result.push(this.value());
        }
        return result;
      }
      case MAP: {
        const count = this.size();
        const result = new Map();
        for (let i = 0; i < count; i++) {
          const key = this.value();
          result.set(key, this.value());
        }
        return result;
      }
      default: throw new Error(`unsupported wire type ${type}`);
    }
  }
}

function encodeMethodCall(method, args) {
  const writer = new Writer();
  writer.value(method);
  writer.value(args);
  return writer.finish();
}

function decodeEnvelope(bytes) {
  const reader = new Reader(bytes);
  if (reader.u8() === 0) {
    return reader.value();
  }
  const code = reader.value();
  throw new Error(`go error ${code}: ${reader.value()}`);
}

// The Dart side of the protocol: opaque registry, callbacks and stream sinks.
const dartOpaques = new Map();
const callbacks = new Map();
const streams = new Map();
let nextDartHandle = 0;
let nextStreamHandle = 0;

function registerDartOpaque(value) {
  const handle = ++nextDartHandle;
  dartOpaques.set(handle, value);
  return handle;
}

function registerCallback(target) {
  const handle = registerDartOpaque('callback');
  callbacks.set(handle, target);
  return handle;
}

function registerStream() {
  const handle = ++nextStreamHandle;
  const events = [];
  let resolveDone;
  const done = new Promise((resolve) => {
    resolveDone = resolve;
  });
  streams.set(handle, {events, resolveDone});
  return {handle, events, done};
}

const registry = () => globalThis.__flutterGoBridge;
const fn = (suffix) => {
  const target = registry()?.[`${LIBRARY}${suffix}`];
  if (typeof target !== 'function') {
    throw new Error(`Go Wasm bridge function ${LIBRARY}${suffix} is not registered`);
  }
  return target;
};

const invokeSync = (method, args) => decodeEnvelope(fn('')(encodeMethodCall(method, args)));
const invokeAsync = async (method, args) =>
    decodeEnvelope(await fn('#async')(encodeMethodCall(method, args)));

function attach() {
  const release = (handle) => {
    dartOpaques.delete(handle);
    callbacks.delete(handle);
  };
  const callback = async (request) => {
    const [, handle, args] = new Reader(request).value();
    const writer = new Writer();
    try {
      const target = callbacks.get(handle);
      if (!target) {
        throw new Error(`unknown callback handle ${handle}`);
      }
      writer.value([0, await target(...args)]);
    } catch (error) {
      writer.value([1, 'callback_error', String(error)]);
    }
    return writer.finish();
  };
  const stream = (raw) => {
    const [handle, kind, payload] = new Reader(raw).value();
    const target = streams.get(handle);
    if (!target) {
      return false;
    }
    if (kind === 0) {
      target.events.push(payload);
      return true;
    }
    streams.delete(handle);
    target.resolveDone();
    return true;
  };
  if (fn('#attach')(release, callback, stream) !== true) {
    throw new Error('Go Wasm bridge rejected the attachment');
  }
}

const failures = [];

function check(name, actual, expected) {
  const same = JSON.stringify(actual) === JSON.stringify(expected);
  console.log(`${same ? 'PASS' : 'FAIL'} ${name}: ${JSON.stringify(actual)}`);
  if (!same) {
    failures.push(`${name}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`);
  }
}

async function run() {
  attach();

  // A value struct crosses as a Map, exactly like the Dart encoders emit.
  const shifted = invokeSync('Point.Shift', [new Map([['x', 1], ['y', 2]]), 5]);
  check('value struct', [shifted.get('x'), shifted.get('y')], [6, 7]);

  // GoOpaque: Go keeps the state, the wire carries only an integer handle.
  const counter = invokeSync('NewCounter', []);
  check('opaque handle is an int', typeof counter, 'number');
  invokeSync('Counter.Add', [counter, 4]);
  check('opaque state persists', invokeSync('Counter.Add', [counter, 3]), 7);

  // Interface dispatch through the tagged implementation envelope.
  check('interface', invokeSync('Describe', [[0, new Map([['radius', 6]])]]), 36);

  // DartOpaque: the object never leaves the Dart/JS side.
  const opaque = registerDartOpaque({kept: true});
  check('dart opaque round trip', invokeSync('Keep', [opaque]), opaque);

  // An async call must run on its own goroutine rather than inline: a second
  // call has to complete while the first is still waiting on a Dart reply.
  // If the Go work ran inside the JS->Go call, the event loop could never turn
  // and the first call would deadlock instead of interleaving.
  const gate = {resolve: null};
  const gated = registerCallback(
    (value) => new Promise((resolve) => {
      gate.resolve = () => resolve(value * 2);
    }),
  );
  const blocked = invokeAsync('Invoke', [gated, 20]);
  let blockedSettled = false;
  blocked.then(() => {
    blockedSettled = true;
  }, () => {
    blockedSettled = true;
  });
  check('second call completes while the first waits', await invokeAsync('SlowDouble', [21]), 42);
  check('first call is still pending', blockedSettled, false);
  gate.resolve();
  check('interleaved callback result', await blocked, 41);

  // A throwing callback must surface as a Go error instead of hanging.
  const failing = registerCallback(async () => {
    throw new Error('boom');
  });
  let propagated = false;
  try {
    await invokeAsync('Invoke', [failing, 1]);
  } catch (error) {
    propagated = String(error).includes('callback failed');
  }
  check('callback error propagates', propagated, true);

  const sink = registerStream();
  await invokeAsync('Emit', [3, sink.handle]);
  await sink.done;
  check('stream sink events', sink.events, [0, 10, 20]);

  const channel = registerStream();
  await invokeAsync('EmitChannel', [4, channel.handle]);
  await channel.done;
  check('channel stream events', channel.events, [0, 100, 200, 300]);

  check('drop opaque handle', fn('#drop')(counter), true);
  let rejected = false;
  try {
    invokeSync('Counter.Add', [counter, 1]);
  } catch (error) {
    rejected = String(error).includes('unknown or released handle');
  }
  check('dropped handle is rejected', rejected, true);

  // Re-attaching stands in for a Dart hot restart. The promise the previous
  // isolate owed Go can never settle, so the producer blocked on it has to be
  // retired rather than left waiting forever. Keep this last: attaching resets
  // the generation and clears every handle registered above.
  const orphaned = registerCallback(() => new Promise(() => {}));
  const stranded = invokeAsync('Invoke', [orphaned, 7]);
  attach();
  let retired = false;
  try {
    await stranded;
  } catch (error) {
    retired = String(error).includes('retired Dart isolate');
  }
  check('re-attach retires an in-flight callback', retired, true);
}

await import(pathToFileURL(path.join(buildDir, 'wasm_exec.js')).href);

const go = new globalThis.Go();
const wasm = await WebAssembly.instantiate(
  fs.readFileSync(path.join(buildDir, 'bridge.wasm')),
  go.importObject,
);
go.run(wasm.instance);

for (let attempt = 0; attempt < 200 && !registry()?.[LIBRARY]; attempt++) {
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (!registry()?.[LIBRARY]) {
  throw new Error(`Go Wasm bridge ${LIBRARY} never registered`);
}

await run();

if (failures.length !== 0) {
  console.error(`\n${failures.length} failure(s):\n${failures.join('\n')}`);
  process.exit(1);
}
process.exit(0);
