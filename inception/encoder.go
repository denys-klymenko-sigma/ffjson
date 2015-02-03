/**
 *  Copyright 2014 Paul Querna
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package ffjsoninception

import (
	"fmt"
	"reflect"
)

func typeInInception(ic *Inception, typ reflect.Type) bool {
	for _, v := range ic.objs {
		if v.Typ == typ {
			return true
		}
		if typ.Kind() == reflect.Ptr {
			if v.Typ == typ.Elem() {
				return true
			}
		}
	}

	return false
}

func getOmitEmpty(ic *Inception, sf *StructField) string {
	ptname := "mj." + sf.Name
	if sf.Pointer {
		ptname = "*" + ptname
	}
	switch sf.Typ.Kind() {

	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return "if len(" + ptname + ") != 0 {" + "\n"

	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64:
		return "if " + ptname + " != 0 {" + "\n"

	case reflect.Bool:
		return "if " + ptname + " != false {" + "\n"

	case reflect.Interface, reflect.Ptr:
		return "if " + ptname + " != nil {" + "\n"

	default:
		// TODO(pquerna): fix types
		return "if true {" + "\n"
	}
}

func getGetInnerValue(ic *Inception, name string, typ reflect.Type, ptr bool, forceString bool) string {
	var out = ""

	// Flush if not bool
	if typ.Kind() != reflect.Bool {
		out += ic.q.Flush()
	}

	if typ.Implements(marshalerFasterType) ||
		reflect.PtrTo(typ).Implements(marshalerFasterType) ||
		typeInInception(ic, typ) ||
		typ.Implements(marshalerType) ||
		reflect.PtrTo(typ).Implements(marshalerType) {

		out += tplStr(encodeTpl["handleMarshaler"], handleMarshaler{
			IC:             ic,
			Name:           name,
			MarshalJSONBuf: typ.Implements(marshalerFasterType) || reflect.PtrTo(typ).Implements(marshalerFasterType) || typeInInception(ic, typ),
			Marshaler:      typ.Implements(marshalerType) || reflect.PtrTo(typ).Implements(marshalerType),
		})
		return out
	}

	ptname := name
	if ptr {
		ptname = "*" + name
	}

	switch typ.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		ic.OutputImports[`fflib "github.com/pquerna/ffjson/fflib/v1"`] = true
		out += "fflib.FormatBits(&scratch, buf, uint64(" + ptname + "), 10, " + ptname + " < 0)" + "\n"
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		ic.OutputImports[`fflib "github.com/pquerna/ffjson/fflib/v1"`] = true
		out += "fflib.FormatBits(&scratch, buf, uint64(" + ptname + "), 10, false)" + "\n"
	case reflect.Float32:
		ic.OutputImports[`"strconv"`] = true
		out += "buf.Write(strconv.AppendFloat([]byte{}, float64(" + ptname + "), 'f', -1, 32))" + "\n"
	case reflect.Float64:
		ic.OutputImports[`"strconv"`] = true
		out += "buf.Write(strconv.AppendFloat([]byte{}, " + ptname + ", 'f', -1, 64))" + "\n"
	case reflect.Array,
		reflect.Slice:
		out += "if " + name + "!= nil {" + "\n"
		out += "buf.WriteString(`[`)" + "\n"
		out += "for i, v := range " + name + "{" + "\n"
		out += "if i != 0 {" + "\n"
		out += "buf.WriteString(`,`)" + "\n"
		out += "}" + "\n"
		out += getGetInnerValue(ic, "v", typ.Elem(), false, false)
		out += "}" + "\n"
		out += "buf.WriteString(`]`)" + "\n"
		out += "} else {" + "\n"
		out += "buf.WriteString(`null`)" + "\n"
		out += "}" + "\n"
	case reflect.String:
		ic.OutputImports[`fflib "github.com/pquerna/ffjson/fflib/v1"`] = true
		if forceString {
			out += "fflib.WriteJsonString(buf, string(" + ptname + `) + "\"")` + "\n"
		} else {
			out += "fflib.WriteJsonString(buf, string(" + ptname + "))" + "\n"
		}
	case reflect.Ptr:
		out += "if " + name + "!= nil {" + "\n"
		switch typ.Elem().Kind() {
		case reflect.Struct:
			out += getGetInnerValue(ic, name, typ.Elem(), false, false)
		default:
			out += getGetInnerValue(ic, "*"+name, typ.Elem(), false, false)
		}
		out += "} else {" + "\n"
		out += "buf.WriteString(`null`)" + "\n"
		out += "}" + "\n"
	case reflect.Bool:
		out += "if " + ptname + " {" + "\n"
		ic.q.Write("true")
		out += ic.q.GetQueued()
		out += "} else {" + "\n"
		// Delete 'true'
		ic.q.DeleteLast()
		out += ic.q.WriteFlush("false")
		out += "}" + "\n"
	case reflect.Interface:
		ic.OutputImports[`"encoding/json"`] = true
		out += fmt.Sprintf("/* Interface types must use runtime reflection. type=%v kind=%v */\n", typ, typ.Kind())
		out += "obj, err = json.Marshal(" + name + ")" + "\n"
		out += "if err != nil {" + "\n"
		out += "  return err" + "\n"
		out += "}" + "\n"
		out += "buf.Write(obj)" + "\n"
	default:
		ic.OutputImports[`"encoding/json"`] = true
		out += fmt.Sprintf("/* Falling back. type=%v kind=%v */\n", typ, typ.Kind())
		out += "obj, err = json.Marshal(" + name + ")" + "\n"
		out += "if err != nil {" + "\n"
		out += "  return err" + "\n"
		out += "}" + "\n"
		out += "buf.Write(obj)" + "\n"
	}

	return out
}

func getValue(ic *Inception, sf *StructField) string {
	if sf.ForceString && !sf.Pointer {
		switch sf.Typ.Kind() {
		case reflect.Int,
			reflect.Int8,
			reflect.Int16,
			reflect.Int32,
			reflect.Int64,
			reflect.Uint,
			reflect.Uint8,
			reflect.Uint16,
			reflect.Uint32,
			reflect.Uint64,
			reflect.Uintptr,
			reflect.Float32,
			reflect.Float64,
			reflect.Bool:
			ic.q.Write(`"`)
			defer ic.q.Write(`"`)
		case reflect.String:
			ic.q.Write(`"\`)
		}
	}
	return getGetInnerValue(ic, "mj."+sf.Name, sf.Typ, sf.Pointer, sf.ForceString)
}

func p2(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func getTypeSize(t reflect.Type) uint32 {
	switch t.Kind() {
	case reflect.String:
		// TODO: consider runtime analysis.
		return 32
	case reflect.Array, reflect.Map, reflect.Slice:
		// TODO: consider runtime analysis.
		return 4 * getTypeSize(t.Elem())
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32:
		return 8
	case reflect.Int64,
		reflect.Uint64,
		reflect.Uintptr:
		return 16
	case reflect.Float32,
		reflect.Float64:
		return 16
	case reflect.Bool:
		return 4
	case reflect.Ptr:
		return getTypeSize(t.Elem())
	default:
		return 16
	}
}

func getTotalSize(si *StructInfo) uint32 {
	rv := uint32(si.Typ.Size())
	for _, f := range si.Fields {
		rv += getTypeSize(f.Typ)
	}
	return rv
}

func getBufGrowSize(si *StructInfo) uint32 {

	// TOOD(pquerna): automatically calc a better grow size based on history
	// of a struct.
	return p2(getTotalSize(si))
}

func isIntish(t reflect.Type) bool {
	if t.Kind() >= reflect.Int && t.Kind() <= reflect.Uintptr {
		return true
	}
	if t.Kind() == reflect.Array || t.Kind() == reflect.Slice || t.Kind() == reflect.Ptr {
		return isIntish(t.Elem())
	}
	return false
}

func CreateMarshalJSON(ic *Inception, si *StructInfo) error {
	conditionalWrites := false
	needScratch := false
	out := ""

	ic.OutputImports[`"bytes"`] = true

	out += `func (mj *` + si.Name + `) MarshalJSON() ([]byte, error) {` + "\n"
	out += `var buf fflib.Buffer` + "\n"

	out += fmt.Sprintf("buf.Grow(%d)\n", getBufGrowSize(si))
	out += `err := mj.MarshalJSONBuf(&buf)` + "\n"
	out += `if err != nil {` + "\n"
	out += "  return nil, err" + "\n"
	out += `}` + "\n"
	out += `return buf.Bytes(), nil` + "\n"
	out += `}` + "\n"

	for _, f := range si.Fields {
		if isIntish(f.Typ) {
			needScratch = true
		}
	}

	// We check if the last field is conditional.
	if len(si.Fields) > 0 {
		f := si.Fields[len(si.Fields)-1]
		conditionalWrites = f.OmitEmpty
	}

	out += `func (mj *` + si.Name + `) MarshalJSONBuf(buf fflib.EncodingBuffer) (error) {` + "\n"
	out += `var err error` + "\n"
	out += `var obj []byte` + "\n"
	if needScratch {
		out += `var scratch fflib.FormatBitsScratch` + "\n"
	}

	out += `_ = obj` + "\n"
	out += `_ = err` + "\n"

	ic.q.Write("{")

	// The extra space is inserted here.
	// If nothing is written to the field this will be deleted
	// instead of the last comma.
	if conditionalWrites || len(si.Fields) == 0 {
		ic.q.Write(" ")
	}

	for _, f := range si.Fields {
		if f.OmitEmpty {
			out += ic.q.Flush()
			if f.Pointer {
				out += "if mj." + f.Name + " != nil {" + "\n"
			}
			out += getOmitEmpty(ic, f)
		}

		if f.Pointer && !f.OmitEmpty {
			// Pointer values encode as the value pointed to. A nil pointer encodes as the null JSON object.
			out += "if mj." + f.Name + " != nil {" + "\n"
		}

		// JsonName is already escaped and quoted.
		// getInnervalue should flush
		ic.q.Write(f.JsonName + ":")
		// We save a copy in case we need it
		t := ic.q

		out += getValue(ic, f)
		ic.q.Write(",")

		if f.Pointer && !f.OmitEmpty {
			out += "} else {" + "\n"
			out += t.WriteFlush("null")
			out += "}" + "\n"
		}

		if f.OmitEmpty {
			if f.Pointer {
				out += "}" + "\n"
			}
			out += ic.q.Flush()
			out += "}" + "\n"
		}
	}

	// Handling the last comma is tricky.
	// If the last field has omitempty, conditionalWrites is set.
	// If something has been written, we delete the last comma,
	// by backing up the buffer, otherwise it will delete a space.
	if conditionalWrites {
		out += ic.q.Flush()
		out += `buf.Rewind(1)` + "\n"
	} else {
		ic.q.DeleteLast()
	}

	out += ic.q.WriteFlush("}")
	out += `return nil` + "\n"
	out += `}` + "\n"
	ic.OutputFuncs = append(ic.OutputFuncs, out)
	return nil
}
