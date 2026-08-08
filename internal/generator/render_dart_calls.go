package generator

import (
	"fmt"
	"strconv"
	"strings"
)

// renderCentralCallMethods keeps all transport and serialization details in
// bridge_generated.dart. Per-source Dart files are deliberately just the
// user-facing API surface and delegate to these methods.
func (r *splitDartRenderer) renderCentralCallMethods() {
	if len(r.unit.Calls) == 0 {
		return
	}
	r.line("extension FgbGeneratedCalls on __FGB_BRIDGE_CLASS__ {")
	for _, call := range r.unit.Calls {
		r.renderCentralCall(call)
	}
	r.line("}")
	r.line("")
}

func (r *splitDartRenderer) renderCentralCall(call *callModel) {
	resultType := dartWireResultType(call)
	params := make([]string, 0, len(call.Params)+1)
	arguments := make([]string, 0, len(call.Params)+1)
	if call.Receiver != nil {
		receiverType := strings.TrimSuffix(call.Receiver.DartType, "?")
		params = append(params, fmt.Sprintf("%s receiver", receiverType))
		arguments = append(arguments, "receiver")
	}
	for _, param := range call.Params {
		params = append(params, fmt.Sprintf("%s %s", dartParamType(param), param.DartName))
		arguments = append(arguments, param.DartName)
	}
	paramText := strings.Join(params, ", ")
	r.line("")
	if isAsyncCall(call) {
		r.line("Future<%s> fgbInternalCall%d(%s) async {", resultType, call.ID, paramText)
	} else {
		r.line("%s fgbInternalCall%d(%s) {", resultType, call.ID, paramText)
	}
	if !call.TargetAvailable {
		reason := call.TargetReason
		if reason == "" {
			reason = "method is unavailable for the selected target"
		}
		r.line("  throw UnsupportedError(%s);", strconv.Quote("Go method "+call.GoName+" is unavailable on Web: "+reason))
		r.line("}")
		return
	}

	// CST/DCO generation is emitted by render_dart_cst.go. Until a type is
	// proven representable by both directions, use the stable Standard codec.
	if r.unit.Target != "web" && call.usesCstDco() {
		r.renderCentralCstCall(call, arguments)
	} else {
		r.renderCentralStandardCall(call, arguments)
	}
	r.line("}")
}

func (r *splitDartRenderer) renderCentralStandardCall(call *callModel, arguments []string) {
	encoded := make([]string, 0, len(arguments))
	argIndex := 0
	if call.Receiver != nil {
		encoded = append(encoded, fmt.Sprintf("fgbEncode%d(receiver, this, %s, 0)", call.Receiver.ID, strconv.Quote("receiver")))
		argIndex++
	}
	for _, param := range call.Params {
		encoded = append(encoded, fmt.Sprintf("fgbEncode%d(%s, this, %s, 0)", param.Type.ID, param.DartName, strconv.Quote(param.DartName)))
		argIndex++
	}
	_ = argIndex
	request := "<Object?>[" + strings.Join(encoded, ", ") + "]"
	if len(call.Results) != 0 {
		if isAsyncCall(call) {
			r.line("  final wireResult = await fgbInvokeAsync(%s, %s);", strconv.Quote(call.WireName), request)
		} else {
			r.line("  final wireResult = fgbInvokeSync(%s, %s);", strconv.Quote(call.WireName), request)
		}
		for _, line := range dartResultDecodeLines(call, "wireResult", "  ") {
			r.line("%s", line)
		}
	} else if isAsyncCall(call) {
		r.line("  await fgbInvokeAsync(%s, %s);", strconv.Quote(call.WireName), request)
	} else {
		r.line("  fgbInvokeSync(%s, %s);", strconv.Quote(call.WireName), request)
	}
}
