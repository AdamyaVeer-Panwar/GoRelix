package relational

import (
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

var (
	ErrInvalidColumnName = errors.New("invalid column name")
	ErrInvalidDataType   = errors.New("invalid data type")
	ErrInvalidSchema     = errors.New("invalid schema")
	ErrDuplicateColumn   = errors.New("duplicate column name")
	ErrInvalidRow        = errors.New("invalid row")
	ErrTypeMismatch      = errors.New("value type does not match column type")
	ErrNullNotAllowed    = errors.New("null value is not allowed")
	ErrInvalidValue      = errors.New("invalid value")
)

type DataType uint8

const (
	TypeInvalid DataType = iota
	TypeInt64
	TypeFloat64
	TypeBool
	TypeString
)

func (t DataType) String() string {
	switch t {
	case TypeInt64:
		return "INT64"
	case TypeFloat64:
		return "FLOAT64"
	case TypeBool:
		return "BOOL"
	case TypeString:
		return "STRING"
	default:
		return "INVALID"
	}
}

func (t DataType) Valid() bool {
	switch t {
	case TypeInt64, TypeFloat64, TypeBool, TypeString:
		return true
	default:
		return false
	}
}

// Column describes one logical field in a relational schema.
type Column struct {
	name     string
	dataType DataType
	nullable bool
}

func NewColumn(name string, dataType DataType, nullable bool) (Column, error) {
	if name == "" {
		return Column{}, fmt.Errorf(
			"%w: name cannot be empty",
			ErrInvalidColumnName,
		)
	}

	if !utf8.ValidString(name) {
		return Column{}, fmt.Errorf(
			"%w: name must be valid UTF-8",
			ErrInvalidColumnName,
		)
	}

	if !dataType.Valid() {
		return Column{}, fmt.Errorf(
			"%w: %d",
			ErrInvalidDataType,
			dataType,
		)
	}

	return Column{
		name:     name,
		dataType: dataType,
		nullable: nullable,
	}, nil
}

func (c Column) Name() string {
	return c.name
}

func (c Column) DataType() DataType {
	return c.dataType
}

func (c Column) Nullable() bool {
	return c.nullable
}

// Schema describes the ordered columns of a logical relation.
//
// Its fields are private so callers cannot mutate the schema after
// construction without passing through validation.
type Schema struct {
	columns []Column
}

func NewSchema(columns ...Column) (Schema, error) {
	if len(columns) == 0 {
		return Schema{}, fmt.Errorf(
			"%w: at least one column is required",
			ErrInvalidSchema,
		)
	}

	seen := make(map[string]struct{}, len(columns))
	copied := make([]Column, len(columns))

	for i, column := range columns {
		if column.name == "" || !utf8.ValidString(column.name) {
			return Schema{}, fmt.Errorf(
				"%w: column %d has an invalid name",
				ErrInvalidSchema,
				i,
			)
		}

		if !column.dataType.Valid() {
			return Schema{}, fmt.Errorf(
				"%w: column %q has an invalid data type",
				ErrInvalidSchema,
				column.name,
			)
		}

		if _, exists := seen[column.name]; exists {
			return Schema{}, fmt.Errorf(
				"%w: %q",
				ErrDuplicateColumn,
				column.name,
			)
		}

		seen[column.name] = struct{}{}
		copied[i] = column
	}

	return Schema{columns: copied}, nil
}

func (s Schema) Len() int {
	return len(s.columns)
}

func (s Schema) Columns() []Column {
	return append([]Column(nil), s.columns...)
}

func (s Schema) ColumnAt(index int) (Column, bool) {
	if index < 0 || index >= len(s.columns) {
		return Column{}, false
	}

	return s.columns[index], true
}

func (s Schema) ColumnByName(name string) (Column, bool) {
	for _, column := range s.columns {
		if column.name == name {
			return column, true
		}
	}

	return Column{}, false
}

// Value is a typed logical value.
//
// Its fields are private to prevent callers from constructing inconsistent
// combinations of type and payload.
type Value struct {
	kind   DataType
	isNull bool

	int64Value   int64
	float64Value float64
	boolValue    bool
	stringValue  string
}

func NullValue() Value {
	return Value{isNull: true}
}

func Int64Value(value int64) Value {
	return Value{
		kind:       TypeInt64,
		int64Value: value,
	}
}

func Float64Value(value float64) Value {
	return Value{
		kind:         TypeFloat64,
		float64Value: value,
	}
}

func BoolValue(value bool) Value {
	return Value{
		kind:      TypeBool,
		boolValue: value,
	}
}

func StringValue(value string) Value {
	return Value{
		kind:        TypeString,
		stringValue: value,
	}
}

func (v Value) IsNull() bool {
	return v.isNull
}

func (v Value) Type() DataType {
	if v.isNull {
		return TypeInvalid
	}

	return v.kind
}

func (v Value) Int64() (int64, bool) {
	if v.isNull || v.kind != TypeInt64 {
		return 0, false
	}

	return v.int64Value, true
}

func (v Value) Float64() (float64, bool) {
	if v.isNull || v.kind != TypeFloat64 {
		return 0, false
	}

	return v.float64Value, true
}

func (v Value) Bool() (bool, bool) {
	if v.isNull || v.kind != TypeBool {
		return false, false
	}

	return v.boolValue, true
}

func (v Value) StringValue() (string, bool) {
	if v.isNull || v.kind != TypeString {
		return "", false
	}

	return v.stringValue, true
}

// Row represents an ordered collection of logical values.
//
// A Row does not know its schema. Schema conformance is checked explicitly
// by Schema.ValidateRow.
type Row struct {
	values []Value
}

func NewRow(values ...Value) Row {
	return Row{
		values: append([]Value(nil), values...),
	}
}

func (r Row) Len() int {
	return len(r.values)
}

func (r Row) Values() []Value {
	return append([]Value(nil), r.values...)
}

func (r Row) ValueAt(index int) (Value, bool) {
	if index < 0 || index >= len(r.values) {
		return Value{}, false
	}

	return r.values[index], true
}

// ValidateRow checks whether a logical row conforms to this schema.
//
// Values must match their declared column types exactly.
// No implicit type conversion is performed.
func (s Schema) ValidateRow(row Row) error {
	if len(s.columns) == 0 {
		return fmt.Errorf(
			"%w: schema is empty",
			ErrInvalidSchema,
		)
	}

	if len(row.values) != len(s.columns) {
		return fmt.Errorf(
			"%w: expected %d values, got %d",
			ErrInvalidRow,
			len(s.columns),
			len(row.values),
		)
	}

	for i, column := range s.columns {
		value := row.values[i]

		if value.isNull {
			if !column.nullable {
				return fmt.Errorf(
					"%w: column %q at index %d",
					ErrNullNotAllowed,
					column.name,
					i,
				)
			}

			continue
		}

		if !value.kind.Valid() {
			return fmt.Errorf(
				"%w: column %q contains an invalid value",
				ErrInvalidValue,
				column.name,
			)
		}

		if value.kind != column.dataType {
			return fmt.Errorf(
				"%w: column %q expects %s, got %s",
				ErrTypeMismatch,
				column.name,
				column.dataType,
				value.kind,
			)
		}

		if value.kind == TypeFloat64 &&
			(math.IsNaN(value.float64Value) ||
				math.IsInf(value.float64Value, 0)) {
			return fmt.Errorf(
				"%w: column %q requires a finite FLOAT64",
				ErrInvalidValue,
				column.name,
			)
		}

		if value.kind == TypeString &&
			!utf8.ValidString(value.stringValue) {
			return fmt.Errorf(
				"%w: column %q contains invalid UTF-8",
				ErrInvalidValue,
				column.name,
			)
		}
	}

	return nil
}
