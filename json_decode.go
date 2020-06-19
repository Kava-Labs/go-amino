package amino

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"github.com/davecgh/go-spew/spew"
)

//----------------------------------------
// cdc.decodeReflectJSON

// CONTRACT: rv.CanAddr() is true.
func (cdc *Codec) decodeReflectJSON(bz []byte, info *TypeInfo, rv reflect.Value, fopts FieldOptions) (err error) {
	if !rv.CanAddr() {
		panic("rv not addressable")
	}
	if info.Type.Kind() == reflect.Interface && rv.Kind() == reflect.Ptr {
		panic("should not happen")
	}
	if printLog {
		spew.Printf("(D) decodeReflectJSON(bz: %s, info: %v, rv: %#v (%v), fopts: %v)\n",
			bz, info, rv.Interface(), rv.Type(), fopts)
		defer func() {
			fmt.Printf("(D) -> err: %v\n", err)
		}()
	}

	// Special case for null for either interface, pointer, slice
	// NOTE: This doesn't match the binary implementation completely.
	if nullBytes(bz) {
		rv.Set(reflect.Zero(rv.Type()))
		return
	}

	// Dereference-and-construct pointers all the way.
	// This works for pointer-pointers.
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			newPtr := reflect.New(rv.Type().Elem())
			rv.Set(newPtr)
		}
		rv = rv.Elem()
	}

	// Handle the most special case, "well known".
	if info.ConcreteInfo.IsJSONWellKnownType {
		var ok bool
		ok, err = decodeReflectJSONWellKnown(bz, info, rv, fopts)
		if ok || err != nil {
			return
		}
	}

	// Handle override if a pointer to rv implements UnmarshalAmino.
	if info.IsAminoUnmarshaler {
		// First, decode repr instance from bytes.
		rrv := reflect.New(info.AminoUnmarshalReprType).Elem()
		var rinfo *TypeInfo
		rinfo, err = cdc.getTypeInfoWLock(info.AminoUnmarshalReprType)
		if err != nil {
			return
		}
		err = cdc.decodeReflectJSON(bz, rinfo, rrv, fopts)
		if err != nil {
			return
		}
		// Then, decode from repr instance.
		uwrm := rv.Addr().MethodByName("UnmarshalAmino")
		uwouts := uwrm.Call([]reflect.Value{rrv})
		erri := uwouts[0].Interface()
		if erri != nil {
			err = erri.(error)
		}
		return
	}

	switch ikind := info.Type.Kind(); ikind {

	//----------------------------------------
	// Complex

	case reflect.Interface:
		err = cdc.decodeReflectJSONInterface(bz, info, rv, fopts)

	case reflect.Array:
		err = cdc.decodeReflectJSONArray(bz, info, rv, fopts)

	case reflect.Slice:
		err = cdc.decodeReflectJSONSlice(bz, info, rv, fopts)

	case reflect.Struct:
		err = cdc.decodeReflectJSONStruct(bz, info, rv, fopts)

	//----------------------------------------
	// Signed, Unsigned

	case reflect.Int64, reflect.Int:
		fallthrough
	case reflect.Uint64, reflect.Uint:
		if bz[0] != '"' || bz[len(bz)-1] != '"' {
			err = errors.Errorf(
				"invalid character -- Amino:JSON int/int64/uint/uint64 expects quoted values for javascript numeric support, got: %v", // nolint: lll
				string(bz),
			)
			if err != nil {
				return
			}
		}
		bz = bz[1 : len(bz)-1]
		fallthrough
	case reflect.Int32, reflect.Int16, reflect.Int8,
		reflect.Uint32, reflect.Uint16, reflect.Uint8:
		err = invokeStdlibJSONUnmarshal(bz, rv, fopts)

	//----------------------------------------
	// Misc

	case reflect.Float32, reflect.Float64:
		if !fopts.Unsafe {
			return errors.New("amino:JSON float* support requires `amino:\"unsafe\"`")
		}
		fallthrough
	case reflect.Bool, reflect.String:
		err = invokeStdlibJSONUnmarshal(bz, rv, fopts)

	//----------------------------------------
	// Default

	default:
		panic(fmt.Sprintf("unsupported type %v", info.Type.Kind()))
	}

	return err
}

func invokeStdlibJSONUnmarshal(bz []byte, rv reflect.Value, fopts FieldOptions) error {
	if !rv.CanAddr() && rv.Kind() != reflect.Ptr {
		panic("rv not addressable nor pointer")
	}

	rrv := rv
	if rv.Kind() != reflect.Ptr {
		rrv = reflect.New(rv.Type())
	}

	if err := json.Unmarshal(bz, rrv.Interface()); err != nil {
		return err
	}
	rv.Set(rrv.Elem())
	return nil
}

// CONTRACT: rv.CanAddr() is true.
func (cdc *Codec) decodeReflectJSONInterface(bz []byte, iinfo *TypeInfo, rv reflect.Value,
	fopts FieldOptions) (err error) {
	if !rv.CanAddr() {
		panic("rv not addressable")
	}
	if printLog {
		fmt.Println("(d) decodeReflectJSONInterface")
		defer func() {
			fmt.Printf("(d) -> err: %v\n", err)
		}()
	}

	/*
		We don't make use of user-provided interface values because there are a
		lot of edge cases.

		* What if the type is mismatched?
		* What if the JSON field entry is missing?
		* Circular references?
	*/
	if !rv.IsNil() {
		// We don't strictly need to set it nil, but lets keep it here for a
		// while in case we forget, for defensive purposes.
		rv.Set(iinfo.ZeroValue)
	}

	// Extract type_url.
	typeURL, value, err := extractJSONTypeURL(bz)
	if err != nil {
		return
	}

	// NOTE: Unlike decodeReflectBinaryInterface, we already dealt with nil in decodeReflectJSON.

	// Get concrete type info.
	// NOTE: Unlike decodeReflectBinaryInterface, uses the full type_url string,
	// which if generated by Amino, is the name preceded by a single slash.
	var cinfo *TypeInfo
	cinfo, err = cdc.getTypeInfoFromTypeURLRLock(typeURL, fopts)
	if err != nil {
		return
	}

	// Extract the value bytes.
	if cinfo.IsJSONAnyValueType {
		bz = value
	} else {
		bz, err = deriveJSONObject(bz, typeURL)
		if err != nil {
			return
		}
	}

	// Construct the concrete type.
	var crv, irvSet = constructConcreteType(cinfo)

	// Decode into the concrete type.
	err = cdc.decodeReflectJSON(bz, cinfo, crv, fopts)
	if err != nil {
		rv.Set(irvSet) // Helps with debugging
		return
	}

	// We need to set here, for when !PointerPreferred and the type
	// is say, an array of bytes (e.g. [32]byte), then we must call
	// rv.Set() *after* the value was acquired.
	rv.Set(irvSet)
	return err
}

// CONTRACT: rv.CanAddr() is true.
func (cdc *Codec) decodeReflectJSONArray(bz []byte, info *TypeInfo, rv reflect.Value, fopts FieldOptions) (err error) {
	if !rv.CanAddr() {
		panic("rv not addressable")
	}
	if printLog {
		fmt.Println("(d) decodeReflectJSONArray")
		defer func() {
			fmt.Printf("(d) -> err: %v\n", err)
		}()
	}
	ert := info.Type.Elem()
	length := info.Type.Len()

	switch ert.Kind() {

	case reflect.Uint8: // Special case: byte array
		var buf []byte
		err = json.Unmarshal(bz, &buf)
		if err != nil {
			return
		}
		if len(buf) != length {
			err = fmt.Errorf("decodeReflectJSONArray: byte-length mismatch, got %v want %v",
				len(buf), length)
		}
		reflect.Copy(rv, reflect.ValueOf(buf))
		return

	default: // General case.
		var einfo *TypeInfo
		einfo, err = cdc.getTypeInfoWLock(ert)
		if err != nil {
			return
		}

		// Read into rawSlice.
		var rawSlice []json.RawMessage
		if err = json.Unmarshal(bz, &rawSlice); err != nil {
			return
		}
		if len(rawSlice) != length {
			err = fmt.Errorf("decodeReflectJSONArray: length mismatch, got %v want %v", len(rawSlice), length)
			return
		}

		// Decode each item in rawSlice.
		for i := 0; i < length; i++ {
			erv := rv.Index(i)
			ebz := rawSlice[i]
			err = cdc.decodeReflectJSON(ebz, einfo, erv, fopts)
			if err != nil {
				return
			}
		}
		return
	}
}

// CONTRACT: rv.CanAddr() is true.
func (cdc *Codec) decodeReflectJSONSlice(bz []byte, info *TypeInfo, rv reflect.Value, fopts FieldOptions) (err error) {
	if !rv.CanAddr() {
		panic("rv not addressable")
	}
	if printLog {
		fmt.Println("(d) decodeReflectJSONSlice")
		defer func() {
			fmt.Printf("(d) -> err: %v\n", err)
		}()
	}

	var ert = info.Type.Elem()

	switch ert.Kind() {

	case reflect.Uint8: // Special case: byte slice
		err = json.Unmarshal(bz, rv.Addr().Interface())
		if err != nil {
			return
		}
		if rv.Len() == 0 {
			// Special case when length is 0.
			// NOTE: We prefer nil slices.
			rv.Set(info.ZeroValue)
		}
		// else {
		// NOTE: Already set via json.Unmarshal() above.
		// }
		return

	default: // General case.
		var einfo *TypeInfo
		einfo, err = cdc.getTypeInfoWLock(ert)
		if err != nil {
			return
		}

		// Read into rawSlice.
		var rawSlice []json.RawMessage
		if err = json.Unmarshal(bz, &rawSlice); err != nil {
			return
		}

		// Special case when length is 0.
		// NOTE: We prefer nil slices.
		var length = len(rawSlice)
		if length == 0 {
			rv.Set(info.ZeroValue)
			return
		}

		// Read into a new slice.
		var esrt = reflect.SliceOf(ert) // TODO could be optimized.
		var srv = reflect.MakeSlice(esrt, length, length)
		for i := 0; i < length; i++ {
			erv := srv.Index(i)
			ebz := rawSlice[i]
			err = cdc.decodeReflectJSON(ebz, einfo, erv, fopts)
			if err != nil {
				return
			}
		}

		// TODO do we need this extra step?
		rv.Set(srv)
		return
	}
}

// CONTRACT: rv.CanAddr() is true.
func (cdc *Codec) decodeReflectJSONStruct(bz []byte, info *TypeInfo, rv reflect.Value, fopts FieldOptions) (err error) {
	if !rv.CanAddr() {
		panic("rv not addressable")
	}
	if printLog {
		fmt.Println("(d) decodeReflectJSONStruct")
		defer func() {
			fmt.Printf("(d) -> err: %v\n", err)
		}()
	}

	// Map all the fields(keys) to their blobs/bytes.
	// NOTE: In decodeReflectBinaryStruct, we don't need to do this,
	// since fields are encoded in order.
	var rawMap = make(map[string]json.RawMessage)
	err = json.Unmarshal(bz, &rawMap)
	if err != nil {
		return
	}

	for _, field := range info.Fields {

		// Get field rv and info.
		var frv = rv.Field(field.Index)
		var finfo *TypeInfo
		finfo, err = cdc.getTypeInfoWLock(field.Type)
		if err != nil {
			return
		}

		// Get value from rawMap.
		var valueBytes = rawMap[field.JSONName]
		if len(valueBytes) == 0 {
			// TODO: Since the Go stdlib's JSON codec allows case-insensitive
			// keys perhaps we need to also do case-insensitive lookups here.
			// So "Vanilla" and "vanilla" would both match to the same field.
			// It is actually a security flaw with encoding/json library
			// - See https://github.com/golang/go/issues/14750
			// but perhaps we are aiming for as much compatibility here.
			// JAE: I vote we depart from encoding/json, than carry a vuln.

			// Set to the zero value only if not omitempty
			if !field.JSONOmitEmpty {
				// Set nil/zero on frv.
				frv.Set(reflect.Zero(frv.Type()))
			}

			continue
		}

		// Decode into field rv.
		err = cdc.decodeReflectJSON(valueBytes, finfo, frv, fopts)
		if err != nil {
			return
		}
	}

	return nil
}

//----------------------------------------
// Misc.

type anyWrapper struct {
	TypeURL string          `json:"@type"`
	Value   json.RawMessage `json:"value"`
}

func extractJSONTypeURL(bz []byte) (typeURL string, value json.RawMessage, err error) {
	anyw := new(anyWrapper)
	err = json.Unmarshal(bz, anyw)
	if err != nil {
		err = fmt.Errorf("cannot parse Any JSON wrapper: %v", err)
		return
	}

	// Get typeURL.
	if anyw.TypeURL == "" {
		err = errors.New("JSON encoding of interfaces require non-empty @type field")
		return
	}
	typeURL = anyw.TypeURL
	value = anyw.Value
	return
}

func deriveJSONObject(bz []byte, typeURL string) (res []byte, err error) {
	str := string(bz)
	if len(bz) == 0 {
		err = errors.New("expected JSON object but was empty")
		return
	}
	if !strings.HasPrefix(str, "{") {
		err = fmt.Errorf("expected JSON object but was not: %s", bz)
		return
	}
	str = strings.TrimLeft(str, " \t\r\n")
	if !strings.HasPrefix(str, `"@type"`) {
		err = fmt.Errorf("expected JSON object representing Any to start with \"@type\" field, but got %v", string(bz))
		return
	}
	str = str[7:]
	str = strings.TrimLeft(str, " \t\r\n")
	if !strings.HasPrefix(str, ":") {
		err = fmt.Errorf("expected JSON object representing Any to start with \"@type\" field, but got %v", string(bz))
		return
	}
	str = str[1:]
	str = strings.TrimLeft(str, " \t\r\n")
	if !strings.HasPrefix(str, fmt.Sprintf(`"%v"`, typeURL)) {
		err = fmt.Errorf("expected JSON object representing Any to start with \"@type\":\"%v\", but got %v", typeURL, string(bz))
		return
	}
	str = str[2+len(typeURL):]
	str = strings.TrimLeft(str, ",")
	return []byte("{" + str), nil
}

func nullBytes(b []byte) bool {
	return bytes.Equal(b, []byte(`null`))
}

func unquoteString(in string) (out string, err error) {
	err = json.Unmarshal([]byte(in), &out)
	return out, err
}
