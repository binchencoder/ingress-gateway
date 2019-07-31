package descriptor

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	gogen "github.com/golang/protobuf/protoc-gen-go/generator"

	// "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/httprule"
	"github.com/binchencoder/ease-gateway/gateway/options"
	"github.com/binchencoder/ease-gateway/gateway/protoc-gen-grpc-gateway/httprule"
	"github.com/binchencoder/ease-gateway/proto/data"
)

// IsWellKnownType returns true if the provided fully qualified type name is considered 'well-known'.
func IsWellKnownType(typeName string) bool {
	_, ok := wellKnownTypeConv[typeName]
	return ok
}

// GoPackage represents a golang package
type GoPackage struct {
	// Path is the package path to the package.
	Path string
	// Name is the package name of the package
	Name string
	// Alias is an alias of the package unique within the current invokation of grpc-gateway generator.
	Alias string
}

// Standard returns whether the import is a golang standard package.
func (p GoPackage) Standard() bool {
	return !strings.Contains(p.Path, ".")
}

// String returns a string representation of this package in the form of import line in golang.
func (p GoPackage) String() string {
	if p.Alias == "" {
		return fmt.Sprintf("%q", p.Path)
	}
	return fmt.Sprintf("%s %q", p.Alias, p.Path)
}

// File wraps descriptor.FileDescriptorProto for richer features.
type File struct {
	*descriptor.FileDescriptorProto
	// GoPkg is the go package of the go file generated from this file..
	GoPkg GoPackage
	// Messages is the list of messages defined in this file.
	Messages []*Message
	// Enums is the list of enums defined in this file.
	Enums []*Enum
	// Services is the list of services defined in this file.
	Services []*Service
}

// proto2 determines if the syntax of the file is proto2.
func (f *File) proto2() bool {
	return f.Syntax == nil || f.GetSyntax() == "proto2"
}

// Message describes a protocol buffer message types
type Message struct {
	// File is the file where the message is defined
	File *File
	// Outers is a list of outer messages if this message is a nested type.
	Outers []string
	*descriptor.DescriptorProto
	Fields []*Field

	// Index is proto path index of this message in File.
	Index int

	// Checked fields, with key constructed with message's package, and field type,
	// And value as the HasRule result.
	// To avoid deadloop in HasRule(). Example problem messages:
	// message A {
	//   B value = 1;
	// }
	// message B {
	//   map<string, A> value = 1;
	// }
	CheckedFields map[string]bool
}

// GetValidationMethodName returns the validation method name of
// this message if exists.
func (m *Message) GetValidationMethodName() string {
	// Validate_{{$message.FQMN | $message.GoName}}
	var components []string
	components = append(m.Outers, "Validate_"+m.GoName())
	name := strings.Join(components, "_")

	return name
}

// GetValidationMethodQualifiedName returns the validation method name of this
// message with package qualifier if not in the current package
func (m *Message) GetValidationMethodQualifiedName(currentPackage string) string {
	name := m.GetValidationMethodName()
	if m.File.GoPkg.Path == currentPackage {
		return name
	}
	pkg := m.File.GoPkg.Name
	if alias := m.File.GoPkg.Alias; alias != "" {
		pkg = alias
	}
	return fmt.Sprintf("%s.%s", pkg, name)
}

// FQMN returns a fully qualified message name of this message.
func (m *Message) FQMN() string {
	components := []string{""}
	if m.File.Package != nil {
		components = append(components, m.File.GetPackage())
	}
	components = append(components, m.Outers...)
	components = append(components, m.GetName())
	return strings.Join(components, ".")
}

// HasRule returns true if there is rule defined for any of the
// field recursively.
func (m *Message) HasRule() bool {
	for _, f := range m.Fields {
		if *f.Type == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
			// It looks the field's type name is prefixed with package already.
			// To make it consistent with parameters passing to Reg.LookupMessage,
			// let's construct map key with package and type name.
			key := m.File.GetPackage() + ":" + f.GetTypeName()

			if hasRule, ok := m.CheckedFields[key]; ok {
				if hasRule {
					return true
				}
				// Skip checked field.
				continue
			}

			m.CheckedFields[key] = false

			if ft, err := Reg.LookupMsg(m.File.GetPackage(), f.GetTypeName()); err == nil {
				if ft != m && ft.HasRule() {
					m.CheckedFields[key] = true
					return true
				}
			}
		}
	}

	return false
}

// GoName convert th FQMN to valid go unique name for the message
// This will can be used to build method/variable name.
func (m *Message) GoName() string {
	return strings.Replace(m.FQMN(), ".", "_", -1)
}

// GoType returns a go type name for the message type.
// It prefixes the type name with the package alias if
// its belonging package is not "currentPackage".
func (m *Message) GoType(currentPackage string) string {
	var components []string
	components = append(components, m.Outers...)
	components = append(components, m.GetName())

	name := strings.Join(components, "_")
	if m.File.GoPkg.Path == currentPackage {
		return name
	}
	pkg := m.File.GoPkg.Name
	if alias := m.File.GoPkg.Alias; alias != "" {
		pkg = alias
	}
	return fmt.Sprintf("%s.%s", pkg, name)
}

// Enum describes a protocol buffer enum types
type Enum struct {
	// File is the file where the enum is defined
	File *File
	// Outers is a list of outer messages if this enum is a nested type.
	Outers []string
	*descriptor.EnumDescriptorProto

	Index int
}

// FQEN returns a fully qualified enum name of this enum.
func (e *Enum) FQEN() string {
	components := []string{""}
	if e.File.Package != nil {
		components = append(components, e.File.GetPackage())
	}
	components = append(components, e.Outers...)
	components = append(components, e.GetName())
	return strings.Join(components, ".")
}

// GoType returns a go type name for the enum type.
// It prefixes the type name with the package alias if
// its belonging package is not "currentPackage".
func (e *Enum) GoType(currentPackage string) string {
	var components []string
	components = append(components, e.Outers...)
	components = append(components, e.GetName())

	name := strings.Join(components, "_")
	if e.File.GoPkg.Path == currentPackage {
		return name
	}
	pkg := e.File.GoPkg.Name
	if alias := e.File.GoPkg.Alias; alias != "" {
		pkg = alias
	}
	return fmt.Sprintf("%s.%s", pkg, name)
}

// Service wraps descriptor.ServiceDescriptorProto for richer features.
type Service struct {
	// File is the file where this service is defined.
	File *File
	*descriptor.ServiceDescriptorProto

	// service ID uniquely identifies this service in context of org.
	ServiceId *data.ServiceId

	// The port name of the service.
	PortName *string

	// The namespece of the service.
	Namespace *string

	GenController bool
	Balancer      options.LoadBalancer

	// Methods is the list of methods defined in this service.
	Methods []*Method
}

// FQSN returns the fully qualified service name of this service.
func (s *Service) FQSN() string {
	components := []string{""}
	if s.File.Package != nil {
		components = append(components, s.File.GetPackage())
	}
	components = append(components, s.GetName())
	return strings.Join(components, ".")
}

// Method wraps descriptor.MethodDescriptorProto for richer features.
type Method struct {
	// Service is the service which this method belongs to.
	Service *Service
	*descriptor.MethodDescriptorProto

	// RequestType is the message type of requests to this method.
	RequestType *Message
	// ResponseType is the message type of responses from this method.
	ResponseType *Message
	Bindings     []*Binding

	LoginRequired      bool
	ClientSignRequired bool
	IsThirdParty       bool
	ApiSource          options.ApiSourceType
	TokenType          options.AuthTokenType
	SpecSourceType     options.SpecSourceType
	HashKey            string
	Timeout            string
}

// FQMN returns a fully qualified rpc method name of this method.
func (m *Method) FQMN() string {
	components := []string{}
	components = append(components, m.Service.FQSN())
	components = append(components, m.GetName())
	return strings.Join(components, ".")
}

// Binding describes how an HTTP endpoint is bound to a gRPC method.
type Binding struct {
	// Method is the method which the endpoint is bound to.
	Method *Method
	// Index is a zero-origin index of the binding in the target method
	Index int
	// PathTmpl is path template where this method is mapped to.
	PathTmpl httprule.Template
	// HTTPMethod is the HTTP method which this method is mapped to.
	HTTPMethod string
	// PathParams is the list of parameters provided in HTTP request paths.
	PathParams []Parameter
	// Body describes parameters provided in HTTP request body.
	Body *Body
	// ResponseBody describes field in response struct to marshal in HTTP response body.
	ResponseBody *Body
}

// ExplicitParams returns a list of explicitly bound parameters of "b",
// i.e. a union of field path for body and field paths for path parameters.
func (b *Binding) ExplicitParams() []string {
	var result []string
	if b.Body != nil {
		result = append(result, b.Body.FieldPath.String())
	}
	for _, p := range b.PathParams {
		result = append(result, p.FieldPath.String())
	}
	return result
}

// Field wraps descriptor.FieldDescriptorProto for richer features.
type Field struct {
	// Message is the message type which this field belongs to.
	Message *Message
	// FieldMessage is the message type of the field.
	FieldMessage *Message
	*descriptor.FieldDescriptorProto

	Rules []*Rule
}

// IsOneOf return true if this field is oneof field.
func (f *Field) IsOneOf() bool {
	return f.OneofIndex != nil
}

// OneOfDeclGoName returns the camel case oneof decl name of this field.
func (f *Field) OneOfDeclGoName() string {
	if f.OneofIndex == nil {
		return ""
	}
	dc := f.Message.OneofDecl[*f.OneofIndex]

	var parts []string
	for _, s := range strings.Split(*dc.Name, "_") {
		part := []rune(s)
		part[0] = unicode.ToUpper(part[0])
		parts = append(parts, string(part))
	}

	return strings.Join(parts, "")
}

// IsRepeated return true if this field is repeated otherwise return false.
func (f *Field) IsRepeated() bool {
	return *f.Label == descriptor.FieldDescriptorProto_LABEL_REPEATED
}

// HasRule returns true if there is any validation rule defined.
func (f *Field) HasRule() bool {
	return len(f.Rules) > 0
}

// GoName returns the field name used by xx.pb.go
func (f *Field) GoName() string {
	if len(*f.JsonName) > 0 {
		json := []rune(*f.JsonName)
		json[0] = unicode.ToUpper(json[0])
		return string(json)
	}

	return ""
}

// Rule wraps options.ValidationRule for richer features
type Rule struct {
	rule *options.ValidationRule
}

// Rule returns the rule
func (r *Rule) Rule() *options.ValidationRule {
	return r.rule
}

// Value returns the value of the rule
func (r *Rule) Value() string {
	return r.rule.Value
}

// IsOpEq returns true if the operator is eq
func (r *Rule) IsOpEq() bool {
	return r.rule.Operator == options.OperatorType_EQ
}

// IsOpGt returns true if the operator is gt
func (r *Rule) IsOpGt() bool {
	return r.rule.Operator == options.OperatorType_GT
}

// IsOpLt returns true if the operator is lt
func (r *Rule) IsOpLt() bool {
	return r.rule.Operator == options.OperatorType_LT
}

// IsOpMatch returns true if the operator is match
func (r *Rule) IsOpMatch() bool {
	return r.rule.Operator == options.OperatorType_MATCH
}

// IsOpNotNil returns true if the operator is not nil
func (r *Rule) IsOpNotNil() bool {
	return r.rule.Operator == options.OperatorType_NON_NIL
}

// IsLenEq returns true if the operator is length eq
func (r *Rule) IsLenEq() bool {
	return r.rule.Operator == options.OperatorType_LEN_EQ
}

// IsLenGt returns true if the operator is length great than
func (r *Rule) IsLenGt() bool {
	return r.rule.Operator == options.OperatorType_LEN_GT
}

// IsLenLt returns true if the operator is length less than
func (r *Rule) IsLenLt() bool {
	return r.rule.Operator == options.OperatorType_LEN_LT
}

// NeedTrim returns true if the trim should be called
// before validation.
func (r *Rule) NeedTrim() bool {
	return r.rule.Function == options.FunctionType_TRIM
}

// IsTypeNumber returns true if the type is number
func (r *Rule) IsTypeNumber() bool {
	return r.rule.Type == options.ValueType_NUMBER
}

// IsTypeObj returns true if the type is object
func (r *Rule) IsTypeObj() bool {
	return r.rule.Type == options.ValueType_OBJ
}

// IsTypeString returns true if the type is string
func (r *Rule) IsTypeString() bool {
	return r.rule.Type == options.ValueType_STRING
}

// Parameter is a parameter provided in http requests
type Parameter struct {
	// FieldPath is a path to a proto field which this parameter is mapped to.
	FieldPath
	// Target is the proto field which this parameter is mapped to.
	Target *Field
	// Method is the method which this parameter is used for.
	Method *Method
}

// ConvertFuncExpr returns a go expression of a converter function.
// The converter function converts a string into a value for the parameter.
func (p Parameter) ConvertFuncExpr() (string, error) {
	tbl := proto3ConvertFuncs
	if !p.IsProto2() && p.IsRepeated() {
		tbl = proto3RepeatedConvertFuncs
	} else if p.IsProto2() && !p.IsRepeated() {
		tbl = proto2ConvertFuncs
	} else if p.IsProto2() && p.IsRepeated() {
		tbl = proto2RepeatedConvertFuncs
	}
	typ := p.Target.GetType()
	conv, ok := tbl[typ]
	if !ok {
		conv, ok = wellKnownTypeConv[p.Target.GetTypeName()]
	}
	if !ok {
		return "", fmt.Errorf("unsupported field type %s of parameter %s in %s.%s", typ, p.FieldPath, p.Method.Service.GetName(), p.Method.GetName())
	}
	return conv, nil
}

// IsEnum returns true if the field is an enum type, otherwise false is returned.
func (p Parameter) IsEnum() bool {
	return p.Target.GetType() == descriptor.FieldDescriptorProto_TYPE_ENUM
}

// IsRepeated returns true if the field is repeated, otherwise false is returned.
func (p Parameter) IsRepeated() bool {
	return p.Target.GetLabel() == descriptor.FieldDescriptorProto_LABEL_REPEATED
}

// IsProto2 returns true if the field is proto2, otherwise false is returned.
func (p Parameter) IsProto2() bool {
	return p.Target.Message.File.proto2()
}

// Body describes a http (request|response) body to be sent to the (method|client).
// This is used in body and response_body options in google.api.HttpRule
type Body struct {
	// FieldPath is a path to a proto field which the (request|response) body is mapped to.
	// The (request|response) body is mapped to the (request|response) type itself if FieldPath is empty.
	FieldPath FieldPath
}

// AssignableExpr returns an assignable expression in Go to be used to initialize method request object.
// It starts with "msgExpr", which is the go expression of the method request object.
func (b Body) AssignableExpr(msgExpr string) string {
	return b.FieldPath.AssignableExpr(msgExpr)
}

// FieldPath is a path to a field from a request message.
type FieldPath []FieldPathComponent

// String returns a string representation of the field path.
func (p FieldPath) String() string {
	var components []string
	for _, c := range p {
		components = append(components, c.Name)
	}
	return strings.Join(components, ".")
}

// IsNestedProto3 indicates whether the FieldPath is a nested Proto3 path.
func (p FieldPath) IsNestedProto3() bool {
	if len(p) > 1 && !p[0].Target.Message.File.proto2() {
		return true
	}
	return false
}

// AssignableExpr is an assignable expression in Go to be used to assign a value to the target field.
// It starts with "msgExpr", which is the go expression of the method request object.
func (p FieldPath) AssignableExpr(msgExpr string) string {
	l := len(p)
	if l == 0 {
		return msgExpr
	}

	var preparations []string
	components := msgExpr
	for i, c := range p {
		// Check if it is a oneOf field.
		if c.Target.OneofIndex != nil {
			index := c.Target.OneofIndex
			msg := c.Target.Message
			oneOfName := gogen.CamelCase(msg.GetOneofDecl()[*index].GetName())
			oneofFieldName := msg.GetName() + "_" + c.AssignableExpr()

			components = components + "." + oneOfName
			s := `if %s == nil {
				%s =&%s{}
			} else if _, ok := %s.(*%s); !ok {
				return nil, metadata, grpc.Errorf(codes.InvalidArgument, "expect type: *%s, but: %%t\n",%s)
			}`

			preparations = append(preparations, fmt.Sprintf(s, components, components, oneofFieldName, components, oneofFieldName, oneofFieldName, components))
			components = components + ".(*" + oneofFieldName + ")"
		}

		if i == l-1 {
			components = components + "." + c.AssignableExpr()
			continue
		}
		components = components + "." + c.ValueExpr()
	}

	preparations = append(preparations, components)
	return strings.Join(preparations, "\n")
}

// FieldPathComponent is a path component in FieldPath
type FieldPathComponent struct {
	// Name is a name of the proto field which this component corresponds to.
	// TODO(yugui) is this necessary?
	Name string
	// Target is the proto field which this component corresponds to.
	Target *Field
}

// AssignableExpr returns an assignable expression in go for this field.
func (c FieldPathComponent) AssignableExpr() string {
	return gogen.CamelCase(c.Name)
}

// ValueExpr returns an expression in go for this field.
func (c FieldPathComponent) ValueExpr() string {
	if c.Target.Message.File.proto2() {
		return fmt.Sprintf("Get%s()", gogen.CamelCase(c.Name))
	}
	return gogen.CamelCase(c.Name)
}

var (
	proto3ConvertFuncs = map[descriptor.FieldDescriptorProto_Type]string{
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:  "runtime.Float64",
		descriptor.FieldDescriptorProto_TYPE_FLOAT:   "runtime.Float32",
		descriptor.FieldDescriptorProto_TYPE_INT64:   "runtime.Int64",
		descriptor.FieldDescriptorProto_TYPE_UINT64:  "runtime.Uint64",
		descriptor.FieldDescriptorProto_TYPE_INT32:   "runtime.Int32",
		descriptor.FieldDescriptorProto_TYPE_FIXED64: "runtime.Uint64",
		descriptor.FieldDescriptorProto_TYPE_FIXED32: "runtime.Uint32",
		descriptor.FieldDescriptorProto_TYPE_BOOL:    "runtime.Bool",
		descriptor.FieldDescriptorProto_TYPE_STRING:  "runtime.String",
		// FieldDescriptorProto_TYPE_GROUP
		// FieldDescriptorProto_TYPE_MESSAGE
		descriptor.FieldDescriptorProto_TYPE_BYTES:    "runtime.Bytes",
		descriptor.FieldDescriptorProto_TYPE_UINT32:   "runtime.Uint32",
		descriptor.FieldDescriptorProto_TYPE_ENUM:     "runtime.Enum",
		descriptor.FieldDescriptorProto_TYPE_SFIXED32: "runtime.Int32",
		descriptor.FieldDescriptorProto_TYPE_SFIXED64: "runtime.Int64",
		descriptor.FieldDescriptorProto_TYPE_SINT32:   "runtime.Int32",
		descriptor.FieldDescriptorProto_TYPE_SINT64:   "runtime.Int64",
	}

	proto3RepeatedConvertFuncs = map[descriptor.FieldDescriptorProto_Type]string{
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:  "runtime.Float64Slice",
		descriptor.FieldDescriptorProto_TYPE_FLOAT:   "runtime.Float32Slice",
		descriptor.FieldDescriptorProto_TYPE_INT64:   "runtime.Int64Slice",
		descriptor.FieldDescriptorProto_TYPE_UINT64:  "runtime.Uint64Slice",
		descriptor.FieldDescriptorProto_TYPE_INT32:   "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_FIXED64: "runtime.Uint64Slice",
		descriptor.FieldDescriptorProto_TYPE_FIXED32: "runtime.Uint32Slice",
		descriptor.FieldDescriptorProto_TYPE_BOOL:    "runtime.BoolSlice",
		descriptor.FieldDescriptorProto_TYPE_STRING:  "runtime.StringSlice",
		// FieldDescriptorProto_TYPE_GROUP
		// FieldDescriptorProto_TYPE_MESSAGE
		descriptor.FieldDescriptorProto_TYPE_BYTES:    "runtime.BytesSlice",
		descriptor.FieldDescriptorProto_TYPE_UINT32:   "runtime.Uint32Slice",
		descriptor.FieldDescriptorProto_TYPE_ENUM:     "runtime.EnumSlice",
		descriptor.FieldDescriptorProto_TYPE_SFIXED32: "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_SFIXED64: "runtime.Int64Slice",
		descriptor.FieldDescriptorProto_TYPE_SINT32:   "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_SINT64:   "runtime.Int64Slice",
	}

	proto2ConvertFuncs = map[descriptor.FieldDescriptorProto_Type]string{
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:  "runtime.Float64P",
		descriptor.FieldDescriptorProto_TYPE_FLOAT:   "runtime.Float32P",
		descriptor.FieldDescriptorProto_TYPE_INT64:   "runtime.Int64P",
		descriptor.FieldDescriptorProto_TYPE_UINT64:  "runtime.Uint64P",
		descriptor.FieldDescriptorProto_TYPE_INT32:   "runtime.Int32P",
		descriptor.FieldDescriptorProto_TYPE_FIXED64: "runtime.Uint64P",
		descriptor.FieldDescriptorProto_TYPE_FIXED32: "runtime.Uint32P",
		descriptor.FieldDescriptorProto_TYPE_BOOL:    "runtime.BoolP",
		descriptor.FieldDescriptorProto_TYPE_STRING:  "runtime.StringP",
		// FieldDescriptorProto_TYPE_GROUP
		// FieldDescriptorProto_TYPE_MESSAGE
		// FieldDescriptorProto_TYPE_BYTES
		// TODO(yugui) Handle bytes
		descriptor.FieldDescriptorProto_TYPE_UINT32:   "runtime.Uint32P",
		descriptor.FieldDescriptorProto_TYPE_ENUM:     "runtime.EnumP",
		descriptor.FieldDescriptorProto_TYPE_SFIXED32: "runtime.Int32P",
		descriptor.FieldDescriptorProto_TYPE_SFIXED64: "runtime.Int64P",
		descriptor.FieldDescriptorProto_TYPE_SINT32:   "runtime.Int32P",
		descriptor.FieldDescriptorProto_TYPE_SINT64:   "runtime.Int64P",
	}

	proto2RepeatedConvertFuncs = map[descriptor.FieldDescriptorProto_Type]string{
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:  "runtime.Float64Slice",
		descriptor.FieldDescriptorProto_TYPE_FLOAT:   "runtime.Float32Slice",
		descriptor.FieldDescriptorProto_TYPE_INT64:   "runtime.Int64Slice",
		descriptor.FieldDescriptorProto_TYPE_UINT64:  "runtime.Uint64Slice",
		descriptor.FieldDescriptorProto_TYPE_INT32:   "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_FIXED64: "runtime.Uint64Slice",
		descriptor.FieldDescriptorProto_TYPE_FIXED32: "runtime.Uint32Slice",
		descriptor.FieldDescriptorProto_TYPE_BOOL:    "runtime.BoolSlice",
		descriptor.FieldDescriptorProto_TYPE_STRING:  "runtime.StringSlice",
		// FieldDescriptorProto_TYPE_GROUP
		// FieldDescriptorProto_TYPE_MESSAGE
		// FieldDescriptorProto_TYPE_BYTES
		// TODO(maros7) Handle bytes
		descriptor.FieldDescriptorProto_TYPE_UINT32:   "runtime.Uint32Slice",
		descriptor.FieldDescriptorProto_TYPE_ENUM:     "runtime.EnumSlice",
		descriptor.FieldDescriptorProto_TYPE_SFIXED32: "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_SFIXED64: "runtime.Int64Slice",
		descriptor.FieldDescriptorProto_TYPE_SINT32:   "runtime.Int32Slice",
		descriptor.FieldDescriptorProto_TYPE_SINT64:   "runtime.Int64Slice",
	}

	wellKnownTypeConv = map[string]string{
		".google.protobuf.Timestamp":   "runtime.Timestamp",
		".google.protobuf.Duration":    "runtime.Duration",
		".google.protobuf.StringValue": "runtime.StringValue",
		".google.protobuf.FloatValue":  "runtime.FloatValue",
		".google.protobuf.DoubleValue": "runtime.DoubleValue",
		".google.protobuf.BoolValue":   "runtime.BoolValue",
		".google.protobuf.BytesValue":  "runtime.BytesValue",
		".google.protobuf.Int32Value":  "runtime.Int32Value",
		".google.protobuf.UInt32Value": "runtime.UInt32Value",
		".google.protobuf.Int64Value":  "runtime.Int64Value",
		".google.protobuf.UInt64Value": "runtime.UInt64Value",
	}
)
