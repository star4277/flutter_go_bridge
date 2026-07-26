package generator

// goCgoPreambleSource is embedded directly in the generated Go file.  This
// keeps the codegen output to one bridge_generated.go while still using the
// Dart API DL C ABI needed by fgb_async and NativeFinalizer.  The helpers are
// static and stateless so cgo may include the preamble in more than one C
// translation unit without creating duplicate global state.
const goCgoPreambleSource = `
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#if defined(__APPLE__)
#include <pthread.h>
#endif

#define FGB_DART_API_DL_MAJOR_VERSION 2

typedef struct {
  uint8_t* data;
  int64_t len;
} FgbData;

// Shared byte/length carrier used by CST strings and hexadecimal integers.
// It is present even when a package currently has no CST call so the stable
// runtime helpers keep compiling across codec-mode changes.
typedef struct {
  uint8_t* ptr;
  int64_t len;
} FgbCstBytes;

typedef enum {
  FgbDartCObjectNull = 0,
  FgbDartCObjectBool,
  FgbDartCObjectInt32,
  FgbDartCObjectInt64,
  FgbDartCObjectDouble,
  FgbDartCObjectString,
  FgbDartCObjectArray,
  FgbDartCObjectTypedData,
  FgbDartCObjectExternalTypedData,
  FgbDartCObjectSendPort,
  FgbDartCObjectCapability,
  FgbDartCObjectNativePointer,
  FgbDartCObjectUnsupported,
  FgbDartCObjectUnmodifiableExternalTypedData
} FgbDartCObjectType;

typedef enum {
  FgbDartTypedDataByteData = 0,
  FgbDartTypedDataInt8,
  FgbDartTypedDataUint8,
  FgbDartTypedDataUint8Clamped,
  FgbDartTypedDataInt16,
  FgbDartTypedDataUint16,
  FgbDartTypedDataInt32,
  FgbDartTypedDataUint32,
  FgbDartTypedDataInt64,
  FgbDartTypedDataUint64,
  FgbDartTypedDataFloat32,
  FgbDartTypedDataFloat64,
  FgbDartTypedDataInt32x4,
  FgbDartTypedDataFloat32x4,
  FgbDartTypedDataFloat64x2,
  FgbDartTypedDataInvalid
} FgbDartTypedDataType;

typedef void (*FgbDartHandleFinalizer)(void* isolate_callback_data, void* peer);

typedef struct FgbDartCObject FgbDartCObject;
struct FgbDartCObject {
  FgbDartCObjectType type;
  union {
    bool as_bool;
    int32_t as_int32;
    int64_t as_int64;
    double as_double;
    const char* as_string;
    struct {
      int64_t id;
      int64_t origin_id;
    } as_send_port;
    struct {
      int64_t id;
    } as_capability;
    struct {
      intptr_t length;
      FgbDartCObject** values;
    } as_array;
    struct {
      FgbDartTypedDataType type;
      intptr_t length;
      const uint8_t* values;
    } as_typed_data;
    struct {
      FgbDartTypedDataType type;
      intptr_t length;
      uint8_t* data;
      void* peer;
      FgbDartHandleFinalizer callback;
    } as_external_typed_data;
    struct {
      intptr_t ptr;
      intptr_t size;
      FgbDartHandleFinalizer callback;
    } as_native_pointer;
  } value;
};

// Alias the local spelling to the official Dart API concept.  The layout is
// intentionally kept identical to Dart_CObject so Dart_PostCObject can copy
// the graph without any Flutter-specific channel or codec.
typedef FgbDartCObject Dart_CObject;

typedef struct {
  const char* name;
  void (*function)(void);
} FgbDartApiEntry;

typedef struct {
  const int major;
  const int minor;
  const FgbDartApiEntry* const functions;
} FgbDartApi;

typedef bool (*FgbDartPostCObject)(int64_t port, FgbDartCObject* message);

static FgbDartPostCObject fgb_lookup_post_c_object(void* data) {
  if (data == NULL) return NULL;
  const FgbDartApi* api = (const FgbDartApi*)data;
  if (api->major != FGB_DART_API_DL_MAJOR_VERSION) return NULL;
  const FgbDartApiEntry* entry = api->functions;
  while (entry != NULL && entry->name != NULL) {
    if (strcmp(entry->name, "Dart_PostCObject") == 0) {
      return (FgbDartPostCObject)entry->function;
    }
    entry++;
  }
  return NULL;
}

static int32_t fgb_internal_init_dart_api(void* data) {
  return fgb_lookup_post_c_object(data) == NULL ? -2 : 0;
}

static bool fgb_internal_post_bytes(void* api_data, int64_t port, const uint8_t* data, int64_t len) {
  if (len < 0) return false;
  FgbDartPostCObject post = fgb_lookup_post_c_object(api_data);
  if (post == NULL) return false;
  FgbDartCObject message;
  message.type = FgbDartCObjectTypedData;
  message.value.as_typed_data.type = FgbDartTypedDataUint8;
  message.value.as_typed_data.length = (intptr_t)len;
  message.value.as_typed_data.values = data;
  return post(port, &message);
}

static FgbDartCObject* fgb_dco_alloc(void) {
  return (FgbDartCObject*)calloc(1, sizeof(FgbDartCObject));
}

static FgbDartCObject* fgb_dco_null(void) {
  FgbDartCObject* object = fgb_dco_alloc();
  if (object != NULL) object->type = FgbDartCObjectNull;
  return object;
}

static FgbDartCObject* fgb_dco_bool(bool value) {
  FgbDartCObject* object = fgb_dco_alloc();
  if (object != NULL) {
    object->type = FgbDartCObjectBool;
    object->value.as_bool = value;
  }
  return object;
}

static FgbDartCObject* fgb_dco_int32(int32_t value) {
  FgbDartCObject* object = fgb_dco_alloc();
  if (object != NULL) {
    object->type = FgbDartCObjectInt32;
    object->value.as_int32 = value;
  }
  return object;
}

static FgbDartCObject* fgb_dco_int64(int64_t value) {
  FgbDartCObject* object = fgb_dco_alloc();
  if (object != NULL) {
    object->type = FgbDartCObjectInt64;
    object->value.as_int64 = value;
  }
  return object;
}

static FgbDartCObject* fgb_dco_double(double value) {
  FgbDartCObject* object = fgb_dco_alloc();
  if (object != NULL) {
    object->type = FgbDartCObjectDouble;
    object->value.as_double = value;
  }
  return object;
}

static FgbDartCObject* fgb_dco_string(const uint8_t* data, int64_t len) {
  if (len < 0) return NULL;
  FgbDartCObject* object = fgb_dco_alloc();
  if (object == NULL) return NULL;
  char* copy = (char*)malloc((size_t)len + 1u);
  if (copy == NULL) {
    free(object);
    return NULL;
  }
  if (len != 0 && data != NULL) memcpy(copy, data, (size_t)len);
  copy[len] = '\0';
  object->type = FgbDartCObjectString;
  object->value.as_string = copy;
  return object;
}

static FgbDartCObject* fgb_dco_typed_data(FgbDartTypedDataType type, const void* data, int64_t len, size_t element_size) {
  if (len < 0 || element_size == 0) return NULL;
  FgbDartCObject* object = fgb_dco_alloc();
  if (object == NULL) return NULL;
  size_t bytes = (size_t)len * element_size;
  uint8_t* copy = (uint8_t*)malloc(bytes == 0 ? 1u : bytes);
  if (copy == NULL) {
    free(object);
    return NULL;
  }
  if (bytes != 0 && data != NULL) memcpy(copy, data, bytes);
  object->type = FgbDartCObjectTypedData;
  object->value.as_typed_data.type = type;
  object->value.as_typed_data.length = (intptr_t)len;
  object->value.as_typed_data.values = copy;
  return object;
}

static FgbDartCObject* fgb_dco_array_new(int64_t len) {
  if (len < 0) return NULL;
  FgbDartCObject* object = fgb_dco_alloc();
  if (object == NULL) return NULL;
  object->type = FgbDartCObjectArray;
  object->value.as_array.length = (intptr_t)len;
  object->value.as_array.values = (FgbDartCObject**)calloc((size_t)len, sizeof(FgbDartCObject*));
  if (len != 0 && object->value.as_array.values == NULL) {
    free(object);
    return NULL;
  }
  return object;
}

static void fgb_dco_array_set(FgbDartCObject* object, int64_t index, FgbDartCObject* child) {
  if (object == NULL || object->type != FgbDartCObjectArray || index < 0 || index >= object->value.as_array.length) return;
  object->value.as_array.values[index] = child;
}

static void fgb_internal_dco_free(FgbDartCObject* object) {
  if (object == NULL) return;
  switch (object->type) {
    case FgbDartCObjectString:
      free((void*)object->value.as_string);
      break;
    case FgbDartCObjectArray:
      if (object->value.as_array.values != NULL) {
        for (intptr_t i = 0; i < object->value.as_array.length; ++i) {
          fgb_internal_dco_free(object->value.as_array.values[i]);
        }
        free(object->value.as_array.values);
      }
      break;
    case FgbDartCObjectTypedData:
      free((void*)object->value.as_typed_data.values);
      break;
    default:
      break;
  }
  free(object);
}

static bool fgb_internal_post_object(void* api_data, int64_t port, FgbDartCObject* object) {
  FgbDartPostCObject post = fgb_lookup_post_c_object(api_data);
  if (post == NULL || object == NULL) return false;
  return post(port, object);
}

extern void fgb_internal_drop_go(void* handle);

#if defined(__APPLE__)
static void* fgb_drop_thread(void* handle) {
  fgb_internal_drop_go(handle);
  return NULL;
}

static void fgb_drop_impl(void* handle) {
  pthread_t thread;
  if (pthread_create(&thread, NULL, fgb_drop_thread, handle) == 0) {
    pthread_detach(thread);
  } else {
    fgb_internal_drop_go(handle);
  }
}
#else
static void fgb_drop_impl(void* handle) {
  fgb_internal_drop_go(handle);
}
#endif
`
