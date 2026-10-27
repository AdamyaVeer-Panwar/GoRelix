package relational

import (
	"errors"
	"math"
	"testing"
)

func mustColumn(
	t *testing.T,
	name string,
	dataType DataType,
	nullable bool,
) Column {
	t.Helper()

	column, err := NewColumn(name, dataType, nullable)
	if err != nil {
		t.Fatalf("NewColumn failed: %v", err)
	}

	return column
}

func mustSchema(t *testing.T, columns ...Column) Schema {
	t.Helper()

	schema, err := NewSchema(columns...)
	if err != nil {
		t.Fatalf("NewSchema failed: %v", err)
	}

	return schema
}

func TestNewColumnRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		dataType DataType
		wantErr  error
	}{
		{
			name:     "empty name",
			column:   "",
			dataType: TypeInt64,
			wantErr:  ErrInvalidColumnName,
		},
		{
			name:     "invalid type",
			column:   "id",
			dataType: TypeInvalid,
			wantErr:  ErrInvalidDataType,
		},
		{
			name:     "unknown type",
			column:   "id",
			dataType: DataType(255),
			wantErr:  ErrInvalidDataType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewColumn(tt.column, tt.dataType, false)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNewSchemaPreservesColumnOrder(t *testing.T) {
	id := mustColumn(t, "id", TypeInt64, false)
	name := mustColumn(t, "name", TypeString, false)
	active := mustColumn(t, "active", TypeBool, false)

	schema := mustSchema(t, id, name, active)

	if schema.Len() != 3 {
		t.Fatalf("expected 3 columns, got %d", schema.Len())
	}

	first, ok := schema.ColumnAt(0)
	if !ok || first.Name() != "id" {
		t.Fatal("first column should be id")
	}

	second, ok := schema.ColumnAt(1)
	if !ok || second.Name() != "name" {
		t.Fatal("second column should be name")
	}

	third, ok := schema.ColumnAt(2)
	if !ok || third.Name() != "active" {
		t.Fatal("third column should be active")
	}
}

func TestNewSchemaRejectsEmptyAndDuplicateColumns(t *testing.T) {
	_, err := NewSchema()

	if !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected ErrInvalidSchema, got %v", err)
	}

	id1 := mustColumn(t, "id", TypeInt64, false)
	id2 := mustColumn(t, "id", TypeString, false)

	_, err = NewSchema(id1, id2)

	if !errors.Is(err, ErrDuplicateColumn) {
		t.Fatalf("expected ErrDuplicateColumn, got %v", err)
	}
}

func TestSchemaColumnLookup(t *testing.T) {
	id := mustColumn(t, "id", TypeInt64, false)
	name := mustColumn(t, "name", TypeString, true)

	schema := mustSchema(t, id, name)

	found, ok := schema.ColumnByName("name")
	if !ok {
		t.Fatal("expected to find name column")
	}

	if found.DataType() != TypeString || !found.Nullable() {
		t.Fatal("unexpected name column definition")
	}

	if _, ok := schema.ColumnByName("missing"); ok {
		t.Fatal("unexpected result for missing column")
	}

	if _, ok := schema.ColumnAt(-1); ok {
		t.Fatal("negative index should be rejected")
	}

	if _, ok := schema.ColumnAt(schema.Len()); ok {
		t.Fatal("out-of-range index should be rejected")
	}
}

func TestSchemaDefensivelyCopiesColumns(t *testing.T) {
	id := mustColumn(t, "id", TypeInt64, false)
	name := mustColumn(t, "name", TypeString, false)

	input := []Column{id, name}

	schema := mustSchema(t, input...)

	input[0] = name

	if got, _ := schema.ColumnAt(0); got.Name() != "id" {
		t.Fatal("schema changed after input slice was modified")
	}

	exposed := schema.Columns()
	exposed[0] = name

	if got, _ := schema.ColumnAt(0); got.Name() != "id" {
		t.Fatal("schema changed after returned slice was modified")
	}
}

func TestValidateRowAcceptsMatchingValues(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "id", TypeInt64, false),
		mustColumn(t, "name", TypeString, false),
		mustColumn(t, "active", TypeBool, false),
		mustColumn(t, "score", TypeFloat64, false),
		mustColumn(t, "nickname", TypeString, true),
	)

	row := NewRow(
		Int64Value(42),
		StringValue("Adamya"),
		BoolValue(true),
		Float64Value(9.5),
		NullValue(),
	)

	if err := schema.ValidateRow(row); err != nil {
		t.Fatalf("expected valid row, got %v", err)
	}
}

func TestValidateRowRejectsWrongValueCount(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "id", TypeInt64, false),
		mustColumn(t, "name", TypeString, false),
	)

	tests := []struct {
		name string
		row  Row
	}{
		{
			name: "too few values",
			row:  NewRow(Int64Value(1)),
		},
		{
			name: "too many values",
			row: NewRow(
				Int64Value(1),
				StringValue("A"),
				BoolValue(true),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateRow(tt.row)

			if !errors.Is(err, ErrInvalidRow) {
				t.Fatalf("expected ErrInvalidRow, got %v", err)
			}
		})
	}
}

func TestValidateRowRejectsTypeMismatch(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "id", TypeInt64, false),
	)

	err := schema.ValidateRow(
		NewRow(StringValue("not an integer")),
	)

	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("expected ErrTypeMismatch, got %v", err)
	}
}

func TestValidateRowRejectsNullForNonNullableColumn(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "id", TypeInt64, false),
	)

	err := schema.ValidateRow(NewRow(NullValue()))

	if !errors.Is(err, ErrNullNotAllowed) {
		t.Fatalf("expected ErrNullNotAllowed, got %v", err)
	}
}

func TestValidateRowAcceptsNullForNullableColumn(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "nickname", TypeString, true),
	)

	if err := schema.ValidateRow(NewRow(NullValue())); err != nil {
		t.Fatalf("expected nullable column to accept NULL: %v", err)
	}
}

func TestValidateRowAcceptsZeroValues(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "count", TypeInt64, false),
		mustColumn(t, "ratio", TypeFloat64, false),
		mustColumn(t, "enabled", TypeBool, false),
		mustColumn(t, "label", TypeString, false),
	)

	row := NewRow(
		Int64Value(0),
		Float64Value(0),
		BoolValue(false),
		StringValue(""),
	)

	if err := schema.ValidateRow(row); err != nil {
		t.Fatalf("expected zero values to be valid: %v", err)
	}
}

func TestValidateRowRejectsNonFiniteFloats(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "measurement", TypeFloat64, false),
	)

	tests := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateRow(
				NewRow(Float64Value(tt.value)),
			)

			if !errors.Is(err, ErrInvalidValue) {
				t.Fatalf("expected ErrInvalidValue, got %v", err)
			}
		})
	}
}

func TestRowDefensivelyCopiesValues(t *testing.T) {
	input := []Value{
		Int64Value(1),
		StringValue("original"),
	}

	row := NewRow(input...)

	input[0] = Int64Value(99)

	first, ok := row.ValueAt(0)
	if !ok {
		t.Fatal("expected first value")
	}

	got, ok := first.Int64()
	if !ok || got != 1 {
		t.Fatal("row changed after input slice was modified")
	}

	exposed := row.Values()
	exposed[1] = StringValue("modified")

	second, ok := row.ValueAt(1)
	if !ok {
		t.Fatal("expected second value")
	}

	gotString, ok := second.StringValue()
	if !ok || gotString != "original" {
		t.Fatal("row changed after returned slice was modified")
	}
}

func TestValueAccessorsRejectWrongTypes(t *testing.T) {
	value := Int64Value(10)

	if _, ok := value.StringValue(); ok {
		t.Fatal("Int64 value must not be accessible as a string")
	}

	if _, ok := NullValue().Int64(); ok {
		t.Fatal("NULL must not be accessible as an integer")
	}
}

func TestNewColumnRejectsInvalidUTF8Name(t *testing.T) {
	name := string([]byte{0xff})

	_, err := NewColumn(name, TypeString, false)
	if !errors.Is(err, ErrInvalidColumnName) {
		t.Fatalf("expected ErrInvalidColumnName, got %v", err)
	}
}

func TestValidateRowRejectsInvalidUTF8String(t *testing.T) {
	schema := mustSchema(
		t,
		mustColumn(t, "name", TypeString, false),
	)

	invalidString := string([]byte{0xff})
	err := schema.ValidateRow(NewRow(StringValue(invalidString)))

	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue, got %v", err)
	}
}
