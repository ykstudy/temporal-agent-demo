// Package contracts owns the published wire contracts, strict JSON boundary and
// canonical content identities. Structural validation is not authorization.
package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxJSONBytes             = 16 << 20
	MaxJSONDepth             = 128
	MaxJSONNodes             = 500000
	MaxJSONNumberBytes       = 128
	MaxJSONExponent          = 400
	MaxSafeInteger     int64 = 9007199254740991
)

// ParseJSON rejects malformed UTF-8, unpaired UTF-16 escapes, duplicate keys
// (including equivalent escaped keys), trailing data, non-finite numbers and
// unsafe integer literals before either schema validation or hashing. Numbers
// remain json.Number so the validator compares numeric bounds without rounding.
func ParseJSON(data []byte) (any, error) {
	if len(data) == 0 || len(data) > MaxJSONBytes {
		return nil, fmt.Errorf("invalid JSON byte size (limit %d)", MaxJSONBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("invalid JSON UTF-8")
	}
	if err := validateUnicodeEscapes(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	value, err := readValue(decoder, 0, &nodes)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("JSON must contain exactly one value")
	}
	return value, nil
}

func readValue(decoder *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > MaxJSONDepth || *nodes > MaxJSONNodes {
		return nil, fmt.Errorf("JSON nesting or node limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON near byte %d", decoder.InputOffset())
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			value := make(map[string]any)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return nil, fmt.Errorf("invalid JSON object key")
				}
				name, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("JSON object key must be a string")
				}
				if _, exists := value[name]; exists {
					// Never echo arbitrary user keys or values into public errors.
					return nil, fmt.Errorf("duplicate JSON object key")
				}
				child, err := readValue(decoder, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				value[name] = child
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, fmt.Errorf("unterminated JSON object")
			}
			return value, nil
		case '[':
			value := make([]any, 0)
			for decoder.More() {
				child, err := readValue(decoder, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				value = append(value, child)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, fmt.Errorf("unterminated JSON array")
			}
			return value, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter")
		}
	case json.Number:
		if len(token) > MaxJSONNumberBytes {
			return nil, fmt.Errorf("JSON number literal exceeds byte limit")
		}
		if pos := strings.IndexAny(string(token), "eE"); pos >= 0 {
			exponent, err := strconv.ParseInt(string(token)[pos+1:], 10, 32)
			if err != nil || exponent < -MaxJSONExponent || exponent > MaxJSONExponent {
				return nil, fmt.Errorf("JSON number exponent exceeds limit")
			}
		}
		n, err := strconv.ParseFloat(string(token), 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return nil, fmt.Errorf("JSON number is not finite binary64")
		}
		if !strings.ContainsAny(string(token), ".eE") {
			integer, err := strconv.ParseInt(string(token), 10, 64)
			if err != nil || integer > MaxSafeInteger || integer < -MaxSafeInteger {
				return nil, fmt.Errorf("JSON integer exceeds interoperable range")
			}
		}
		return token, nil
	case string, bool, nil:
		return token, nil
	default:
		return nil, fmt.Errorf("unsupported JSON value")
	}
}

// encoding/json intentionally repairs invalid surrogate escapes. Validate those
// escapes in the original bytes first so repaired text can never acquire a hash.
func validateUnicodeEscapes(data []byte) error {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return fmt.Errorf("unfinished JSON escape")
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return fmt.Errorf("unfinished JSON Unicode escape")
		}
		u, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return fmt.Errorf("invalid JSON Unicode escape")
		}
		i += 4
		if u >= 0xDC00 && u <= 0xDFFF {
			return fmt.Errorf("unpaired low Unicode surrogate")
		}
		if u >= 0xD800 && u <= 0xDBFF {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return fmt.Errorf("unpaired high Unicode surrogate")
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return fmt.Errorf("invalid Unicode surrogate pair")
			}
			i += 6
		}
	}
	return nil
}

func jsonBytes(value any) ([]byte, error) {
	if err := validateGoStrings(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	if raw, ok := value.(json.RawMessage); ok {
		return append([]byte(nil), raw...), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("value cannot be encoded as JSON")
	}
	return data, nil
}

func validateGoStrings(value reflect.Value, depth int) error {
	if depth > MaxJSONDepth {
		return fmt.Errorf("Go value nesting exceeds JSON limit")
	}
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("Go string contains invalid UTF-8")
		}
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			return validateGoStrings(value.Elem(), depth+1)
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if err := validateGoStrings(iter.Key(), depth+1); err != nil {
				return err
			}
			if err := validateGoStrings(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < value.Len(); i++ {
			if err := validateGoStrings(value.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				if err := validateGoStrings(value.Field(i), depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func marshalUnion(data []byte) ([]byte, error) {
	if data == nil {
		return []byte("null"), nil
	}
	if _, err := ParseJSON(data); err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

// Decode validates the original bytes before Go decoding can discard fields,
// replace duplicate keys, or confuse a missing required field with its zero value.
func Decode(name string, data []byte, out any) error {
	if err := ValidateJSON(name, data); err != nil {
		return err
	}
	return decodeValidated(name, data, out)
}

// DecodeVersion selects one exact release before decoding; it never upgrades
// old persisted objects or interprets them through a new release's defaults.
func DecodeVersion(name, version string, data []byte, out any) error {
	if err := ValidateJSONVersion(name, version, data); err != nil {
		return err
	}
	return decodeValidated(name, data, out)
}

func decodeValidated(name string, data []byte, out any) error {
	// JSON Schema's integer includes 1.0 and 1e0. Normalize exact safe integral
	// lexemes before the Go int64 decoder, without rounding fractional values or
	// turning unbounded JsonValue numbers into unsafe integer literals.
	value, err := ParseJSON(data)
	if err != nil {
		return err
	}
	normalizeIntegralLexemes(value)
	if number, ok := value.(json.Number); ok {
		value = integralLexeme(number)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cannot normalize validated JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("validated %s cannot be decoded into destination type", name)
	}
	return nil
}

func integralLexeme(number json.Number) json.Number {
	rational, ok := new(big.Rat).SetString(string(number))
	if ok && rational.IsInt() && rational.Num().IsInt64() {
		integer := rational.Num().Int64()
		if integer <= MaxSafeInteger && integer >= -MaxSafeInteger {
			return json.Number(strconv.FormatInt(integer, 10))
		}
	}
	return number
}

func normalizeIntegralLexemes(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if number, ok := item.(json.Number); ok {
				value[key] = integralLexeme(number)
			} else {
				normalizeIntegralLexemes(item)
			}
		}
	case []any:
		for index, item := range value {
			if number, ok := item.(json.Number); ok {
				value[index] = integralLexeme(number)
			} else {
				normalizeIntegralLexemes(item)
			}
		}
	}
}
