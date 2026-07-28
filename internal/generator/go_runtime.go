package generator

const goRuntimeSource = `
const (
	fgbMessageNull = iota
	fgbMessageTrue
	fgbMessageFalse
	fgbMessageInt32
	fgbMessageInt64
	fgbMessageBigInt
	fgbMessageFloat64
	fgbMessageString
	fgbMessageUint8List
	fgbMessageInt32List
	fgbMessageInt64List
	fgbMessageFloat64List
	fgbMessageList
	fgbMessageMap
	fgbMessageFloat32List
)

const (
	fgbEnvelopeSuccess = 0
	fgbEnvelopeError   = 1
)

var fgbEndian binary.ByteOrder = binary.LittleEndian
var fgbHandles sync.Map
var fgbNextHandle atomic.Uintptr
var fgbDartAPIData unsafe.Pointer
var fgbDartOpaquePort atomic.Int64
var fgbCallbackPort atomic.Int64
var fgbCallbackWaiters sync.Map
var fgbNextCallbackCall atomic.Int64
var fgbStreamPort atomic.Int64
var fgbCancelledStreams sync.Map
var fgbStreamCancels sync.Map

func init() {
	var marker uint16 = 0x1
	if *(*byte)(unsafe.Pointer(&marker)) == 0 {
		fgbEndian = binary.BigEndian
	}
}

type fgbMethodCall struct {
	Method    string
	Arguments any
}

type fgbCallError struct {
	Code    string
	Message string
	Details any
}

type fgbStandardMethodCodec struct{}

func (fgbStandardMethodCodec) writeSize(buffer *bytes.Buffer, value int) error {
	if value < 0 {
		return fmt.Errorf("negative message size %d", value)
	}
	switch {
	case value < 254:
		return buffer.WriteByte(byte(value))
	case value <= 0xffff:
		if err := buffer.WriteByte(254); err != nil {
			return err
		}
		var raw [2]byte
		fgbEndian.PutUint16(raw[:], uint16(value))
		_, err := buffer.Write(raw[:])
		return err
	default:
		if uint64(value) > uint64(^uint32(0)) {
			return fmt.Errorf("message size %d exceeds uint32", value)
		}
		if err := buffer.WriteByte(255); err != nil {
			return err
		}
		var raw [4]byte
		fgbEndian.PutUint32(raw[:], uint32(value))
		_, err := buffer.Write(raw[:])
		return err
	}
}

func (fgbStandardMethodCodec) writeAlignment(buffer *bytes.Buffer, alignment int) error {
	padding := (alignment - buffer.Len()%alignment) % alignment
	if padding == 0 {
		return nil
	}
	_, err := buffer.Write(make([]byte, padding))
	return err
}

func (codec fgbStandardMethodCodec) writeBytes(buffer *bytes.Buffer, value []byte) error {
	if err := codec.writeSize(buffer, len(value)); err != nil {
		return err
	}
	_, err := buffer.Write(value)
	return err
}

func (codec fgbStandardMethodCodec) writeString(buffer *bytes.Buffer, value string) error {
	return codec.writeBytes(buffer, []byte(value))
}

func (codec fgbStandardMethodCodec) writeValue(buffer *bytes.Buffer, value any) error {
	if value == nil {
		return buffer.WriteByte(fgbMessageNull)
	}
	switch value := value.(type) {
	case bool:
		if value {
			return buffer.WriteByte(fgbMessageTrue)
		}
		return buffer.WriteByte(fgbMessageFalse)
	case int32:
		if err := buffer.WriteByte(fgbMessageInt32); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case int64:
		if err := buffer.WriteByte(fgbMessageInt64); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case *big.Int:
		if value == nil {
			return buffer.WriteByte(fgbMessageNull)
		}
		if err := buffer.WriteByte(fgbMessageBigInt); err != nil {
			return err
		}
		return codec.writeString(buffer, value.Text(16))
	case float64:
		if err := buffer.WriteByte(fgbMessageFloat64); err != nil {
			return err
		}
		if err := codec.writeAlignment(buffer, 8); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case string:
		if err := buffer.WriteByte(fgbMessageString); err != nil {
			return err
		}
		return codec.writeString(buffer, value)
	case []byte:
		if err := buffer.WriteByte(fgbMessageUint8List); err != nil {
			return err
		}
		return codec.writeBytes(buffer, value)
	case []int32:
		if err := buffer.WriteByte(fgbMessageInt32List); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		if err := codec.writeAlignment(buffer, 4); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case []int64:
		if err := buffer.WriteByte(fgbMessageInt64List); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		if err := codec.writeAlignment(buffer, 8); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case []float32:
		if err := buffer.WriteByte(fgbMessageFloat32List); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		if err := codec.writeAlignment(buffer, 4); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case []float64:
		if err := buffer.WriteByte(fgbMessageFloat64List); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		if err := codec.writeAlignment(buffer, 8); err != nil {
			return err
		}
		return binary.Write(buffer, fgbEndian, value)
	case []any:
		if err := buffer.WriteByte(fgbMessageList); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		for _, item := range value {
			if err := codec.writeValue(buffer, item); err != nil {
				return err
			}
		}
		return nil
	case map[any]any:
		if err := buffer.WriteByte(fgbMessageMap); err != nil {
			return err
		}
		if err := codec.writeSize(buffer, len(value)); err != nil {
			return err
		}
		for key, item := range value {
			if err := codec.writeValue(buffer, key); err != nil {
				return err
			}
			if err := codec.writeValue(buffer, item); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("type %T is not supported by StandardMessageCodec", value)
	}
}

func (fgbStandardMethodCodec) readSize(buffer *bytes.Buffer) (int, error) {
	marker, err := buffer.ReadByte()
	if err != nil {
		return 0, err
	}
	switch marker {
	case 254:
		if buffer.Len() < 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return int(fgbEndian.Uint16(buffer.Next(2))), nil
	case 255:
		if buffer.Len() < 4 {
			return 0, io.ErrUnexpectedEOF
		}
		return int(fgbEndian.Uint32(buffer.Next(4))), nil
	default:
		return int(marker), nil
	}
}

func (fgbStandardMethodCodec) readAlignment(buffer *bytes.Buffer, originalSize, alignment int) error {
	position := originalSize - buffer.Len()
	padding := (alignment - position%alignment) % alignment
	if buffer.Len() < padding {
		return io.ErrUnexpectedEOF
	}
	buffer.Next(padding)
	return nil
}

func (codec fgbStandardMethodCodec) readBytes(buffer *bytes.Buffer) ([]byte, error) {
	length, err := codec.readSize(buffer)
	if err != nil {
		return nil, err
	}
	if length < 0 || buffer.Len() < length {
		return nil, io.ErrUnexpectedEOF
	}
	return append([]byte(nil), buffer.Next(length)...), nil
}

func (codec fgbStandardMethodCodec) readValue(buffer *bytes.Buffer, originalSize int) (any, error) {
	valueType, err := buffer.ReadByte()
	if err != nil {
		return nil, err
	}
	switch valueType {
	case fgbMessageNull:
		return nil, nil
	case fgbMessageTrue:
		return true, nil
	case fgbMessageFalse:
		return false, nil
	case fgbMessageInt32:
		var result int32
		return result, binary.Read(buffer, fgbEndian, &result)
	case fgbMessageInt64:
		var result int64
		return result, binary.Read(buffer, fgbEndian, &result)
	case fgbMessageBigInt:
		raw, err := codec.readBytes(buffer)
		if err != nil {
			return nil, err
		}
		result, ok := new(big.Int).SetString(string(raw), 16)
		if !ok {
			return nil, fmt.Errorf("invalid BigInt encoding")
		}
		return result, nil
	case fgbMessageFloat64:
		if err := codec.readAlignment(buffer, originalSize, 8); err != nil {
			return nil, err
		}
		var result float64
		return result, binary.Read(buffer, fgbEndian, &result)
	case fgbMessageString:
		raw, err := codec.readBytes(buffer)
		return string(raw), err
	case fgbMessageUint8List:
		return codec.readBytes(buffer)
	case fgbMessageInt32List:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		if err := codec.readAlignment(buffer, originalSize, 4); err != nil {
			return nil, err
		}
		result := make([]int32, length)
		return result, binary.Read(buffer, fgbEndian, result)
	case fgbMessageInt64List:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		if err := codec.readAlignment(buffer, originalSize, 8); err != nil {
			return nil, err
		}
		result := make([]int64, length)
		return result, binary.Read(buffer, fgbEndian, result)
	case fgbMessageFloat32List:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		if err := codec.readAlignment(buffer, originalSize, 4); err != nil {
			return nil, err
		}
		result := make([]float32, length)
		return result, binary.Read(buffer, fgbEndian, result)
	case fgbMessageFloat64List:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		if err := codec.readAlignment(buffer, originalSize, 8); err != nil {
			return nil, err
		}
		result := make([]float64, length)
		return result, binary.Read(buffer, fgbEndian, result)
	case fgbMessageList:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		result := make([]any, length)
		for index := range result {
			result[index], err = codec.readValue(buffer, originalSize)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case fgbMessageMap:
		length, err := codec.readSize(buffer)
		if err != nil {
			return nil, err
		}
		result := make(map[any]any, length)
		for index := 0; index < length; index++ {
			key, err := codec.readValue(buffer, originalSize)
			if err != nil {
				return nil, err
			}
			value, err := codec.readValue(buffer, originalSize)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown StandardMessageCodec type %d", valueType)
	}
}

func (codec fgbStandardMethodCodec) decodeMethodCall(data []byte) (fgbMethodCall, error) {
	buffer := bytes.NewBuffer(data)
	originalSize := buffer.Len()
	methodValue, err := codec.readValue(buffer, originalSize)
	if err != nil {
		return fgbMethodCall{}, fmt.Errorf("decode method name: %w", err)
	}
	method, ok := methodValue.(string)
	if !ok {
		return fgbMethodCall{}, fmt.Errorf("method name has type %T, want string", methodValue)
	}
	arguments, err := codec.readValue(buffer, originalSize)
	if err != nil {
		return fgbMethodCall{}, fmt.Errorf("decode arguments: %w", err)
	}
	if buffer.Len() != 0 {
		return fgbMethodCall{}, fmt.Errorf("method call contains %d trailing bytes", buffer.Len())
	}
	return fgbMethodCall{Method: method, Arguments: arguments}, nil
}

func (codec fgbStandardMethodCodec) encodeSuccess(value any) ([]byte, error) {
	var buffer bytes.Buffer
	if err := buffer.WriteByte(fgbEnvelopeSuccess); err != nil {
		return nil, err
	}
	if err := codec.writeValue(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (codec fgbStandardMethodCodec) encodeError(code, message string, details any) ([]byte, error) {
	var buffer bytes.Buffer
	if err := buffer.WriteByte(fgbEnvelopeError); err != nil {
		return nil, err
	}
	for _, value := range []any{code, message, details} {
		if err := codec.writeValue(&buffer, value); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func fgbTypeError(path, expected string, actual any) error {
	return fmt.Errorf("%s: expected %s, got %T", path, expected, actual)
}

func fgbAsInt64(value any, path string) (int64, error) {
	switch value := value.(type) {
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case *big.Int:
		if value.IsInt64() {
			return value.Int64(), nil
		}
		return 0, fmt.Errorf("%s: integer out of int64 range", path)
	default:
		return 0, fgbTypeError(path, "integer", value)
	}
}

func fgbAsUint64(value any, path string) (uint64, error) {
	switch value := value.(type) {
	case int32:
		if value < 0 {
			return 0, fmt.Errorf("%s: expected non-negative integer", path)
		}
		return uint64(value), nil
	case int64:
		if value < 0 {
			return 0, fmt.Errorf("%s: expected non-negative integer", path)
		}
		return uint64(value), nil
	case *big.Int:
		if value.Sign() >= 0 && value.IsUint64() {
			return value.Uint64(), nil
		}
		return 0, fmt.Errorf("%s: integer out of uint64 range", path)
	case string:
		parsed, ok := new(big.Int).SetString(value, 16)
		if !ok || parsed.Sign() < 0 || !parsed.IsUint64() {
			return 0, fmt.Errorf("%s: invalid hexadecimal uint64", path)
		}
		return parsed.Uint64(), nil
	default:
		return 0, fgbTypeError(path, "non-negative integer", value)
	}
}

func fgbAsFloat64(value any, path string) (float64, error) {
	switch value := value.(type) {
	case float64:
		return value, nil
	case int32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	default:
		return 0, fgbTypeError(path, "double", value)
	}
}

func fgbAsHandle(value any, path string) (uintptr, error) {
	raw, err := fgbAsUint64(value, path)
	if err != nil {
		return 0, err
	}
	handle := uintptr(raw)
	if handle == 0 || uint64(handle) != raw {
		return 0, fmt.Errorf("%s: invalid opaque handle %d", path, raw)
	}
	return handle, nil
}

func fgbNormalize(value any, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("dynamic value nesting exceeds 64 levels")
	}
	if value == nil {
		return nil, nil
	}
	switch value := value.(type) {
	case bool, int32, int64, float64, string, []byte, []int32, []int64, []float32, []float64, *big.Int:
		return value, nil
	case int:
		return int64(value), nil
	case int8:
		return int32(value), nil
	case int16:
		return int32(value), nil
	case uint8:
		return int32(value), nil
	case uint16:
		return int32(value), nil
	case uint32:
		return int64(value), nil
	case uint, uint64, uintptr:
		raw := reflect.ValueOf(value).Convert(reflect.TypeOf(uint64(0))).Uint()
		return new(big.Int).SetUint64(raw), nil
	}

	raw := reflect.ValueOf(value)
	for raw.Kind() == reflect.Interface || raw.Kind() == reflect.Pointer {
		if raw.IsNil() {
			return nil, nil
		}
		raw = raw.Elem()
	}
	switch raw.Kind() {
	case reflect.Slice, reflect.Array:
		result := make([]any, raw.Len())
		for index := range result {
			item, err := fgbNormalize(raw.Index(index).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			result[index] = item
		}
		return result, nil
	case reflect.Map:
		result := make(map[any]any, raw.Len())
		iterator := raw.MapRange()
		for iterator.Next() {
			key, err := fgbNormalize(iterator.Key().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			item, err := fgbNormalize(iterator.Value().Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			if key != nil && !reflect.TypeOf(key).Comparable() {
				return nil, fmt.Errorf("dynamic map key %T is not comparable", key)
			}
			result[key] = item
		}
		return result, nil
	default:
		return nil, fmt.Errorf("dynamic value type %T is not supported", value)
	}
}

func fgbRequireArgs(value any, count int) ([]any, error) {
	arguments, ok := value.([]any)
	if !ok {
		return nil, fgbTypeError("arguments", "List", value)
	}
	if len(arguments) != count {
		return nil, fmt.Errorf("expected %d arguments, got %d", count, len(arguments))
	}
	return arguments, nil
}

func fgbInvalidArguments(method string, err error) *fgbCallError {
	return &fgbCallError{
		Code: "invalid_argument", Message: err.Error(),
		Details: map[any]any{"method": method},
	}
}

func fgbGoError(method string, err error) *fgbCallError {
	return &fgbCallError{
		Code: "go_error", Message: err.Error(),
		Details: map[any]any{"method": method},
	}
}

// fgbGoErrors reports every non-nil error of a call that declares several
// error results. The individual messages travel in the details so Dart can
// surface them as FgbGoErrors.
func fgbGoErrors(method string, messages []any) *fgbCallError {
	combined := ""
	for index, message := range messages {
		if index != 0 {
			combined += "; "
		}
		combined += fmt.Sprint(message)
	}
	return &fgbCallError{
		Code: "go_error", Message: combined,
		Details: map[any]any{"method": method, "errors": messages},
	}
}

func fgbEncodeCallError(codec fgbStandardMethodCodec, callErr *fgbCallError) []byte {
	encoded, err := codec.encodeError(callErr.Code, callErr.Message, callErr.Details)
	if err == nil {
		return encoded
	}
	// Strings and nil are always supported, so this is a safe last resort.
	encoded, _ = codec.encodeError("bridge_error", err.Error(), nil)
	return encoded
}

// CST/DCO helpers ---------------------------------------------------------
//
// The generated CST decoders use these small checked primitives rather than
// repeating unsafe length arithmetic for every field.  DCO constructors copy
// their input before returning, so temporary C buffers can be released as
// soon as the helper returns.
func fgbCstLength(length C.int64_t, path string) (int, error) {
	if length < 0 {
		return 0, fmt.Errorf("%s: negative length %d", path, int64(length))
	}
	if uint64(length) > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("%s: length %d exceeds platform int", path, int64(length))
	}
	return int(length), nil
}

func fgbCstReadBytes(data unsafe.Pointer, length C.int64_t, path string) ([]byte, error) {
	n, err := fgbCstLength(length, path)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return []byte{}, nil
	}
	if data == nil {
		return nil, fmt.Errorf("%s: nil data pointer", path)
	}
	return append([]byte(nil), unsafe.Slice((*byte)(data), n)...), nil
}

func fgbCstReadString(data unsafe.Pointer, length C.int64_t, path string) (string, error) {
	raw, err := fgbCstReadBytes(data, length, path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func fgbCstReadBigInt(value *C.FgbCstBytes, path string) (*big.Int, error) {
	if value == nil {
		return nil, fmt.Errorf("%s: null BigInt", path)
	}
	raw, err := fgbCstReadBytes(unsafe.Pointer(value.ptr), value.len, path)
	if err != nil {
		return nil, err
	}
	parsed, ok := new(big.Int).SetString(string(raw), 16)
	if !ok {
		return nil, fmt.Errorf("%s: invalid hexadecimal BigInt", path)
	}
	return parsed, nil
}

func fgbDcoNull() (*C.FgbDartCObject, error) {
	result := C.fgb_dco_null()
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoBool(value bool) (*C.FgbDartCObject, error) {
	result := C.fgb_dco_bool(C.bool(value))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoInt32(value int32) (*C.FgbDartCObject, error) {
	result := C.fgb_dco_int32(C.int32_t(value))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoInt64(value int64) (*C.FgbDartCObject, error) {
	result := C.fgb_dco_int64(C.int64_t(value))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoDouble(value float64) (*C.FgbDartCObject, error) {
	result := C.fgb_dco_double(C.double(value))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoString(value string) (*C.FgbDartCObject, error) {
	for index := 0; index < len(value); index++ {
		if value[index] == 0 {
			return nil, fmt.Errorf("DCO String contains an embedded NUL byte")
		}
	}
	raw := []byte(value)
	var data unsafe.Pointer
	if len(raw) != 0 {
		data = C.CBytes(raw)
		defer C.free(data)
	}
	result := C.fgb_dco_string((*C.uint8_t)(data), C.int64_t(len(raw)))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoBytes(value []byte) (*C.FgbDartCObject, error) {
	return fgbDcoTypedData(C.FgbDartTypedDataUint8, value, 1)
}

func fgbDcoInt32List(value []int32) (*C.FgbDartCObject, error) {
	return fgbDcoTypedData(C.FgbDartTypedDataInt32, value, 4)
}

func fgbDcoInt64List(value []int64) (*C.FgbDartCObject, error) {
	return fgbDcoTypedData(C.FgbDartTypedDataInt64, value, 8)
}

func fgbDcoFloat64List(value []float64) (*C.FgbDartCObject, error) {
	return fgbDcoTypedData(C.FgbDartTypedDataFloat64, value, 8)
}

func fgbDcoTypedData(kind C.FgbDartTypedDataType, value any, elementSize int) (*C.FgbDartCObject, error) {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case []int32:
		if len(typed) != 0 {
			raw = unsafe.Slice((*byte)(unsafe.Pointer(&typed[0])), len(typed)*elementSize)
		}
	case []int64:
		if len(typed) != 0 {
			raw = unsafe.Slice((*byte)(unsafe.Pointer(&typed[0])), len(typed)*elementSize)
		}
	case []float64:
		if len(typed) != 0 {
			raw = unsafe.Slice((*byte)(unsafe.Pointer(&typed[0])), len(typed)*elementSize)
		}
	default:
		return nil, fmt.Errorf("unsupported typed DCO data %T", value)
	}
	var data unsafe.Pointer
	if len(raw) != 0 {
		data = C.CBytes(raw)
		defer C.free(data)
	}
	length := 0
	if elementSize != 0 {
		length = len(raw) / elementSize
	}
	result := C.fgb_dco_typed_data(kind, data, C.int64_t(length), C.size_t(elementSize))
	if result == nil {
		return nil, fmt.Errorf("DCO allocation failed")
	}
	return result, nil
}

func fgbDcoEnvelopeSuccess(value *C.FgbDartCObject) *C.FgbDartCObject {
	root := C.fgb_dco_array_new(2)
	if root == nil {
		if value != nil {
			C.fgb_internal_dco_free(value)
		}
		return nil
	}
	flag, err := fgbDcoInt32(0)
	if err != nil {
		C.fgb_internal_dco_free(root)
		if value != nil {
			C.fgb_internal_dco_free(value)
		}
		return nil
	}
	if value == nil {
		value, err = fgbDcoNull()
		if err != nil {
			C.fgb_internal_dco_free(flag)
			C.fgb_internal_dco_free(root)
			return nil
		}
	}
	C.fgb_dco_array_set(root, 0, flag)
	C.fgb_dco_array_set(root, 1, value)
	return root
}

// fgbDcoErrorDetails carries the individual messages of a multi-error call.
// Dart_CObject has no map type, so the DCO path sends a plain list of strings
// where the standard codec sends the whole details map.
func fgbDcoErrorDetails(callErr *fgbCallError) (*C.FgbDartCObject, error) {
	raw, _ := callErr.Details.(map[any]any)
	messages, _ := raw["errors"].([]any)
	if len(messages) == 0 {
		return fgbDcoNull()
	}
	list := C.fgb_dco_array_new(C.int64_t(len(messages)))
	if list == nil {
		return nil, fmt.Errorf("DCO array allocation failed")
	}
	for index, message := range messages {
		child, err := fgbDcoString(fmt.Sprint(message))
		if err != nil {
			C.fgb_internal_dco_free(list)
			return nil, err
		}
		C.fgb_dco_array_set(list, C.int64_t(index), child)
	}
	return list, nil
}

func fgbDcoEnvelopeError(callErr *fgbCallError) *C.FgbDartCObject {
	if callErr == nil {
		callErr = &fgbCallError{Code: "bridge_error", Message: "unknown bridge error"}
	}
	root := C.fgb_dco_array_new(4)
	if root == nil {
		return nil
	}
	code, err1 := fgbDcoString(callErr.Code)
	message, err2 := fgbDcoString(callErr.Message)
	details, err3 := fgbDcoErrorDetails(callErr)
	if err1 != nil || err2 != nil || err3 != nil {
		C.fgb_internal_dco_free(root)
		if code != nil { C.fgb_internal_dco_free(code) }
		if message != nil { C.fgb_internal_dco_free(message) }
		if details != nil { C.fgb_internal_dco_free(details) }
		return nil
	}
	flag := C.fgb_dco_int32(C.int32_t(1))
	if flag == nil {
		C.fgb_internal_dco_free(root)
		C.fgb_internal_dco_free(code)
		C.fgb_internal_dco_free(message)
		C.fgb_internal_dco_free(details)
		return nil
	}
	C.fgb_dco_array_set(root, 0, flag)
	C.fgb_dco_array_set(root, 1, code)
	C.fgb_dco_array_set(root, 2, message)
	C.fgb_dco_array_set(root, 3, details)
	return root
}

func fgbHandle(request []byte) (response []byte) {
	codec := fgbStandardMethodCodec{}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = fgbEncodeCallError(codec, &fgbCallError{
				Code: "panic", Message: fmt.Sprintf("panic: %v", recovered),
				Details: map[any]any{"stack": debug.Stack()},
			})
		}
	}()
	call, err := codec.decodeMethodCall(request)
	if err != nil {
		return fgbEncodeCallError(codec, &fgbCallError{Code: "invalid_request", Message: err.Error()})
	}
	value, callErr := fgbDispatch(call)
	if callErr != nil {
		return fgbEncodeCallError(codec, callErr)
	}
	response, err = codec.encodeSuccess(value)
	if err != nil {
		return fgbEncodeCallError(codec, &fgbCallError{Code: "encode_error", Message: err.Error()})
	}
	return response
}

func fgbCopyInput(data unsafe.Pointer, length int64) ([]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("negative input length %d", length)
	}
	if uint64(length) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("input length %d exceeds platform int", length)
	}
	if length == 0 {
		return nil, nil
	}
	if data == nil {
		return nil, fmt.Errorf("nil input pointer with length %d", length)
	}
	view := unsafe.Slice((*byte)(data), int(length))
	return append([]byte(nil), view...), nil
}

func fgbErrorResponse(err error) []byte {
	return fgbEncodeCallError(fgbStandardMethodCodec{}, &fgbCallError{Code: "invalid_request", Message: err.Error()})
}

func fgbToCData(data []byte) C.FgbData {
	return C.FgbData{data: (*C.uint8_t)(C.CBytes(data)), len: C.int64_t(len(data))}
}

func fgbPost(port int64, data []byte) {
	var pointer *C.uint8_t
	if len(data) != 0 {
		pointer = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	C.fgb_internal_post_bytes(fgbDartAPIData, C.int64_t(port), pointer, C.int64_t(len(data)))
	runtime.KeepAlive(data)
}

//export fgb_init
func fgb_init(data unsafe.Pointer) C.int32_t {
	status := C.int32_t(C.fgb_internal_init_dart_api(data))
	if status == 0 {
		fgbDartAPIData = data
	}
	return status
}

//export fgb_alloc
func fgb_alloc(size C.int64_t) unsafe.Pointer {
	if size < 0 {
		return nil
	}
	if uint64(size) > uint64(^uintptr(0)) {
		return nil
	}
	// malloc(0) is allowed to return nil. Keep the ABI useful for callers
	// that intentionally pass an empty payload by allocating one harmless byte.
	if size == 0 {
		size = 1
	}
	return C.malloc(C.size_t(size))
}

//export fgb_free
func fgb_free(data unsafe.Pointer) {
	C.free(data)
}

//export fgb
func fgb(data unsafe.Pointer, length C.int64_t) C.FgbData {
	request, err := fgbCopyInput(data, int64(length))
	if err != nil {
		return fgbToCData(fgbErrorResponse(err))
	}
	return fgbToCData(fgbHandle(request))
}

//export fgb_async
func fgb_async(data unsafe.Pointer, length C.int64_t, port C.int64_t) {
	request, err := fgbCopyInput(data, int64(length))
	if err != nil {
		go fgbPost(int64(port), fgbErrorResponse(err))
		return
	}
	// The dispatch must run inside the goroutine: "go f(g(x))" would evaluate
	// g(x) on this thread, keeping the Dart isolate blocked inside the FFI
	// call for the whole computation (and deadlocking any Dart callback).
	go func() {
		fgbPost(int64(port), fgbHandle(request))
	}()
}

func fgbHandleCst(callID int32, args unsafe.Pointer) (response *C.FgbDartCObject) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = fgbDcoEnvelopeError(&fgbCallError{
				Code: "panic", Message: fmt.Sprintf("panic: %v", recovered),
			})
		}
	}()
	payload, callErr := fgbDispatchCst(callID, args)
	if callErr != nil {
		return fgbDcoEnvelopeError(callErr)
	}
	return fgbDcoEnvelopeSuccess(payload)
}

func fgbPostDco(port int64, object *C.FgbDartCObject) {
	if object == nil {
		return
	}
	// Dart_PostCObject copies the graph during this call.  Once it returns the
	// native graph is ours again regardless of delivery success.
	C.fgb_internal_post_object(fgbDartAPIData, C.int64_t(port), object)
	C.fgb_internal_dco_free(object)
}

//export fgb_cst
func fgb_cst(callID C.int32_t, args unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(fgbHandleCst(int32(callID), args))
}

//export fgb_cst_async
func fgb_cst_async(callID C.int32_t, args unsafe.Pointer, port C.int64_t) {
	// Same argument-evaluation caveat as fgb_async: the dispatch itself has to
	// happen inside the goroutine, otherwise the calling Dart isolate stays
	// blocked until the Go function returns. The CST arena stays alive on the
	// Dart side until the reply lands on the port, so reading args here is safe.
	go func() {
		fgbPostDco(int64(port), fgbHandleCst(int32(callID), args))
	}()
}

//export fgb_dco_free
func fgb_dco_free(object unsafe.Pointer) {
	if object != nil {
		C.fgb_internal_dco_free((*C.FgbDartCObject)(object))
	}
}

//export fgb_drop
func fgb_drop(handle unsafe.Pointer) {
	C.fgb_drop_impl(handle)
}

//export fgb_internal_drop_go
func fgb_internal_drop_go(handle unsafe.Pointer) {
	if handle != nil {
		fgbHandles.Delete(uintptr(handle))
	}
}

//export fgb_dart_opaque_port
func fgb_dart_opaque_port(port C.int64_t) {
	fgbDartOpaquePort.Store(int64(port))
}

// fgbReleaseDartOpaque tells the Dart side that the last Go copy of a
// DartOpaque was collected, so its registry entry can be dropped. Used by
// generated decoders via fgb.NewDartOpaque; safe to call from cleanup
// goroutines.
func fgbReleaseDartOpaque(handle int64) {
	port := fgbDartOpaquePort.Load()
	if port == 0 || handle == 0 {
		return
	}
	object, err := fgbDcoInt64(handle)
	if err != nil {
		return
	}
	fgbPostDco(port, object)
}

//export fgb_callback_port
func fgb_callback_port(port C.int64_t) {
	fgbCallbackPort.Store(int64(port))
}

//export fgb_callback_result
func fgb_callback_result(id C.int64_t, data unsafe.Pointer, length C.int64_t) {
	payload, err := fgbCopyInput(data, int64(length))
	if err != nil {
		payload = nil
	}
	if waiter, ok := fgbCallbackWaiters.LoadAndDelete(int64(id)); ok {
		waiter.(chan []byte) <- payload
	}
}

// fgbCallbackRef keeps the Dart-side closure registered for as long as the
// synthesized Go func value is reachable; afterwards the shared DartOpaque
// release path drops the registry entry.
type fgbCallbackRef struct {
	handle int64
}

func fgbNewCallbackRef(handle int64) *fgbCallbackRef {
	ref := &fgbCallbackRef{handle: handle}
	runtime.AddCleanup(ref, fgbReleaseDartOpaque, handle)
	return ref
}

// fgbInvokeCallback posts one callback request to the Dart event loop and
// parks the calling goroutine until the reply arrives. It must therefore only
// run inside //fgb:async calls: during a synchronous call the Dart thread is
// blocked inside the FFI call and could never execute the closure. Arguments
// are pre-encoded standard-codec values; the decoded reply value is returned.
func fgbInvokeCallback(handle int64, arguments []any) (any, error) {
	port := fgbCallbackPort.Load()
	if port == 0 {
		return nil, fmt.Errorf("callback port is not initialized; was the bridge opened via initialize()?")
	}
	id := fgbNextCallbackCall.Add(1)
	waiter := make(chan []byte, 1)
	fgbCallbackWaiters.Store(id, waiter)

	codec := fgbStandardMethodCodec{}
	var buffer bytes.Buffer
	if err := codec.writeValue(&buffer, []any{id, handle, arguments}); err != nil {
		fgbCallbackWaiters.Delete(id)
		return nil, fmt.Errorf("encode callback request: %w", err)
	}
	fgbPost(port, buffer.Bytes())

	payload := <-waiter
	if payload == nil {
		return nil, fmt.Errorf("callback reply was empty")
	}
	replyBuffer := bytes.NewBuffer(payload)
	decoded, err := codec.readValue(replyBuffer, len(payload))
	if err != nil {
		return nil, fmt.Errorf("decode callback reply: %w", err)
	}
	envelope, ok := decoded.([]any)
	if !ok || len(envelope) == 0 {
		return nil, fmt.Errorf("invalid callback reply envelope")
	}
	flag, err := fgbAsInt64(envelope[0], "callback envelope")
	if err != nil {
		return nil, err
	}
	switch {
	case flag == 0 && len(envelope) == 2:
		return envelope[1], nil
	case flag == 1 && len(envelope) >= 3:
		return nil, fmt.Errorf("dart callback failed: %v", envelope[2])
	default:
		return nil, fmt.Errorf("invalid callback reply envelope")
	}
}

//export fgb_stream_port
func fgb_stream_port(port C.int64_t) {
	fgbStreamPort.Store(int64(port))
}

//export fgb_stream_cancel
func fgb_stream_cancel(handle C.int64_t) {
	fgbCancelledStreams.Store(int64(handle), true)
	if cancel, ok := fgbStreamCancels.LoadAndDelete(int64(handle)); ok {
		cancel.(context.CancelFunc)()
	}
}

// fgbRegisterStreamCancel ties a call-scoped context to a stream: when Dart
// stops listening the context is cancelled, which is how a cooperative Go
// producer learns to stop early.
func fgbRegisterStreamCancel(handle int64, cancel context.CancelFunc) {
	if _, cancelled := fgbCancelledStreams.Load(handle); cancelled {
		cancel()
		return
	}
	fgbStreamCancels.Store(handle, cancel)
}

func fgbUnregisterStreamCancel(handle int64) {
	fgbStreamCancels.Delete(handle)
}

// fgbMustStreamHandle reads a stream handle from a decoded argument; an
// invalid handle yields 0, which simply produces a stream nobody listens to.
func fgbMustStreamHandle(value any) int64 {
	handle, err := fgbAsInt64(value, "stream")
	if err != nil {
		return 0
	}
	return handle
}

// fgbPostStreamEvent hands one stream event to the Dart event loop without
// waiting for it to be processed. It reports false once the Dart side stopped
// listening, which the sink turns into fgb.ErrStreamClosed.
func fgbPostStreamEvent(handle int64, kind int32, payload any) bool {
	if _, cancelled := fgbCancelledStreams.Load(handle); cancelled {
		return false
	}
	port := fgbStreamPort.Load()
	if port == 0 {
		return false
	}
	var buffer bytes.Buffer
	if err := (fgbStandardMethodCodec{}).writeValue(&buffer, []any{handle, int64(kind), payload}); err != nil {
		return false
	}
	fgbPost(port, buffer.Bytes())
	return true
}

// fgbReleaseStreamSink runs after Go drops its last copy of a sink: the Dart
// stream is completed so listeners are not left hanging.
func fgbReleaseStreamSink(handle int64) {
	if _, cancelled := fgbCancelledStreams.LoadAndDelete(handle); cancelled {
		return
	}
	fgbPostStreamEvent(handle, 1, nil)
}
`
