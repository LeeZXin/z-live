package amf

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
)

func (e *Encoder) EncodeAmf0(w io.Writer, val any) (int, error) {
	if val == nil {
		return e.EncodeAmf0Null(w, true)
	}
	v := reflect.ValueOf(val)
	if !v.IsValid() {
		return e.EncodeAmf0Null(w, true)
	}
	switch v.Kind() {
	case reflect.String:
		str := v.String()
		if len(str) <= AMF0_STRING_MAX {
			return e.EncodeAmf0String(w, str, true)
		} else {
			return e.EncodeAmf0LongString(w, str, true)
		}
	case reflect.Bool:
		return e.EncodeAmf0Boolean(w, v.Bool(), true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.EncodeAmf0Number(w, float64(v.Int()), true)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.EncodeAmf0Number(w, float64(v.Uint()), true)
	case reflect.Float32, reflect.Float64:
		return e.EncodeAmf0Number(w, float64(v.Float()), true)
	case reflect.Array, reflect.Slice:
		length := v.Len()
		arr := make(Array, length)
		for i := 0; i < length; i++ {
			arr[i] = v.Index(int(i)).Interface()
		}
		return e.EncodeAmf0StrictArray(w, arr, true)
	case reflect.Map:
		obj, ok := val.(Object)
		if ok != true {
			return 0, fmt.Errorf("encode amf0: unable to create object from map")
		}
		return e.EncodeAmf0Object(w, obj, true)
	}
	if _, ok := val.(TypedObject); ok {
		return 0, fmt.Errorf("encode amf0: unsupported type typed object")
	}
	return 0, fmt.Errorf("encode amf0: unsupported type %s", v.Type())
}

func (e *Encoder) EncodeAmf0Number(w io.Writer, val float64, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_NUMBER_MARKER); err != nil {
			return
		}
		n += 1
	}
	err = binary.Write(w, binary.BigEndian, &val)
	if err != nil {
		return
	}
	n += 8
	return
}

func (e *Encoder) EncodeAmf0Boolean(w io.Writer, val bool, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_BOOLEAN_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	buf := make([]byte, 1)
	if val {
		buf[0] = AMF0_BOOLEAN_TRUE
	} else {
		buf[0] = AMF0_BOOLEAN_FALSE
	}
	m, err = w.Write(buf)
	if err != nil {
		return
	}
	n += m
	return
}

func (e *Encoder) EncodeAmf0String(w io.Writer, val string, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_STRING_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	length := uint16(len(val))
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode string length: %s", err)
	}
	n += 2
	m, err = w.Write([]byte(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode string value: %s", err)
	}
	n += m
	return
}

func (e *Encoder) EncodeAmf0Object(w io.Writer, val Object, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_OBJECT_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	for k, v := range val {
		m, err = e.EncodeAmf0String(w, k, false)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode object key: %s", err)
		}
		n += m
		m, err = e.EncodeAmf0(w, v)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode object value: %s", err)
		}
		n += m
	}
	m, err = e.EncodeAmf0String(w, "", false)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode object empty string: %s", err)
	}
	n += m
	err = WriteMarker(w, AMF0_OBJECT_END_MARKER)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to object end marker: %s", err)
	}
	n += 1
	return
}

func (e *Encoder) EncodeAmf0Null(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_NULL_MARKER); err != nil {
			return
		}
		n += 1
	}
	return
}

func (e *Encoder) EncodeAmf0Undefined(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_UNDEFINED_MARKER); err != nil {
			return
		}
		n += 1
	}
	return
}

func (e *Encoder) EncodeAmf0EcmaArray(w io.Writer, val Object, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_ECMA_ARRAY_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	length := uint32(len(val))
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode ecma array length: %s", err)
	}
	n += 4
	m, err = e.EncodeAmf0Object(w, val, false)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode ecma array object: %s", err)
	}
	n += m

	return
}

func (e *Encoder) EncodeAmf0StrictArray(w io.Writer, val Array, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_STRICT_ARRAY_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	length := uint32(len(val))
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode strict array length: %s", err)
	}
	n += 4
	for _, v := range val {
		m, err = e.EncodeAmf0(w, v)
		if err != nil {
			return n, fmt.Errorf("encode amf0: unable to encode strict array element: %s", err)
		}
		n += m
	}
	return
}

func (e *Encoder) EncodeAmf0LongString(w io.Writer, val string, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_LONG_STRING_MARKER); err != nil {
			return
		}
		n += 1
	}
	var m int
	length := uint32(len(val))
	err = binary.Write(w, binary.BigEndian, length)
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode long string length: %s", err)
	}
	n += 4
	m, err = w.Write([]byte(val))
	if err != nil {
		return n, fmt.Errorf("encode amf0: unable to encode long string value: %s", err)
	}
	n += m
	return
}

func (e *Encoder) EncodeAmf0Unsupported(w io.Writer, encodeMarker bool) (n int, err error) {
	if encodeMarker {
		if err = WriteMarker(w, AMF0_UNSUPPORTED_MARKER); err != nil {
			return
		}
		n += 1
	}
	return
}

func (e *Encoder) EncodeAmf0Amf3Marker(w io.Writer) error {
	return WriteMarker(w, AMF0_ACMPLUS_OBJECT_MARKER)
}
