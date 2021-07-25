package dml

import (
	"github.com/DATA-DOG/go-sqlmock"

	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"strings"
)

// Notes:
// some of the stuff this file tests involves reflection, type analysis, and caching types.
// you should make type aliases of any types that you use that you expect to go through the
// cache, especially if you're going to be testing any features that depend on the cache's
// behavior. This is important because tests can run in any order, and if you don't use
// aliases you may find that a previous test contaminated the cache and causes your test to
// fail because the cache is not in the state it expects.  Tests should in general not test
// the cache's behavior outside of the impact that their own specific test has upon it.

// force interface implementation for the following classes
var _ Scannable =         &sql.Row{}
var _ Scannable =          dumbAssFuckinAdapter{sqlRows: (*sql.Rows)(nil)}
var _ AdvancedScannable =  dumbAssFuckinAdapter{sqlRows: (*sql.Rows)(nil)}
var _ IterableScannable =  dumbAssFuckinAdapter{sqlRows: (*sql.Rows)(nil)}
var _ Scannable =         &scannableWrapper{}
var _ AdvancedScannable = &scannableWrapper{}
var _ IterableScannable = &scannableWrapper{}

var constError1 = errors.New("const error 1")

type FakeRow struct {
	scanned []interface{}
}

func (f *FakeRow) Scan(data ...interface{}) error {
	f.scanned = data
	return constError1
}

func Test_scannableWrapper(t *testing.T) {
	inner := &FakeRow{}
	wrapped := WrapBasic(inner)
	wrapper, ok := wrapped.(*scannableWrapper)
	var e error

	if !ok { t.Errorf("Unexpected wrapper object: wanted %s, got %s", reflect.TypeOf(&scannableWrapper{}).Name(), reflect.TypeOf(wrapped).Elem()) }
	if wrapper.Scannable != inner { t.Errorf("Unexpected wrapper state (embedded Scannable): wanted %+v, got %+v", inner, wrapper.Scannable) }
	if e = wrapped.Err(); e != nil { t.Errorf("Unexpected return value (wrapped.Err()): got %+v, expected nil", e) }
	if wrapper.done { t.Errorf("Unexpected wrapper state (done): wanted false, got true") }
	if ok = wrapped.Next(); ok != true { t.Errorf("Unexpected return value (wrapped.Next()): got false, expected true") }
	if e = wrapped.Err(); e != nil { t.Errorf("Unexpected return value (wrapped.Err()): got %+v, expected nil", e) }
	if !wrapper.done { t.Errorf("Unexpected wrapper state (done): wanted true, got false") }
	if ok = wrapped.Next(); ok == true { t.Errorf("Unexpected return value (wrapped.Next()): got true, expected false") }
	if e = wrapped.Err(); e != nil { t.Errorf("Unexpected return value (wrapped.Err()): got %+v, expected nil", e) }
	if !wrapper.done { t.Errorf("Unexpected wrapper state (done): wanted true, got false") }

	// the wrapper should always allow scan, which should always returns the inner error
	values := []interface{}{1,2,3}
	if e = wrapped.Scan(values...); e != constError1 { t.Errorf("Unexpected return value (wrapped.Scan()): got %v, expected %v", e, constError1) }
	if !reflect.DeepEqual(values, inner.scanned) { t.Errorf("Unexpected wrapper state (scanned): got %v, expected %v", inner.scanned, values) }

	n, e := wrapped.ColumnNames()
	if n != nil || e != nil { t.Errorf("Unexpected return value (wrapped.ColumnTypes()): got %v, %v; expected nil, nil", n, e) }
}

func Test_noopScanner(t *testing.T) {
	var s sql.Scanner = noopScanner{}
	if e := s.Scan([]byte("THIS IS A STRING")); e != nil { t.Errorf("Unexpected return value (s.Scan): got %v, expected nil", e) }
}

func Test_iln(t *testing.T) {
	var x *iln
	x = x.add(1)
	x = x.add(5)
	x = x.add(3)

	first,  x := x.yoink()
	second, x := x.yoink()
	third,  x := x.yoink()
	fourth, x := x.yoink()

	if first == nil  || *first  != 1 { t.Errorf("Unexpected return value (x.yoink()): got %v, expected 1", *first) }
	if second == nil || *second != 5 { t.Errorf("Unexpected return value (x.yoink()): got %v, expected 5", *second) }
	if third == nil  || *third  != 3 { t.Errorf("Unexpected return value (x.yoink()): got %v, expected 3", *third) }
	if fourth != nil                 { t.Errorf("Unexpected return value (x.yoink()): got %v, expected nil", fourth) }
}

type E1 struct {
	Test string `dml:"value3"`
}

type E2 struct {
	E1

	Test string `dml:"value4"`
}

type X1 struct {
	E1
	E2

	Test1   string `dml:"value1"`
	Test2   string `dml:"value2"`
	private string `dml:"value5"`
}

type X0 X1
func (x *X0) NoDefaults() {}
func (x *X0) GetFields() (NamedFields, error) {
	return NamedFields{
		Names: []string{"private"},
		Fields: []interface{}{&x.private},
	}, nil
}

func Test_buildNamedFieldsCacheForType(t *testing.T) {
	x := X1{E1: E1{Test: "value3"}, E2: E2{E1: E1{Test: "value3"}, Test: "value4"}, Test1: "value1", Test2: "value2", private: "value4"}
	v_ptr := reflect.ValueOf(&x)
	v_raw := v_ptr.Elem()
	v_type := v_raw.Type()
	tagged_fields := 5

	cache, err := buildFieldCacheEntryForType(v_type, nil)
	if err != nil { t.Errorf("Unexpected return value (buildFieldCacheEntryForType()): got %v, expected nil", err) }
	if len(cache.Names) != tagged_fields { t.Errorf("Unexpected state (cache.Names): wrong length, got %d, expected %d", len(cache.Names), tagged_fields) }
	if len(cache.Fields) != tagged_fields { t.Errorf("Unexpected state (cache.Fields): wrong length, got %d, expected %d", len(cache.Fields), tagged_fields) }
	if len(cache.IsScanner) != tagged_fields { t.Errorf("Unexpected state (cache.IsScanner): wrong length, got %d, expected %d", len(cache.IsScanner), tagged_fields) }
	for i := range cache.Names {
		if []string{"value3", "value3", "value4", "value1", "value2"}[i] != cache.Names[i] {
			t.Errorf("Unexpected field: %s in %+v", cache.Names[i], cache)
		}
		if v_raw.FieldByIndex(cache.Fields[i]).String() != v_type.FieldByIndex(cache.Fields[i]).Tag.Get("dml") {
			t.Errorf("Incorrectly mapped field %s in %+v", cache.Names[i], cache)
		}
	}

	cache, err = buildFieldCacheEntryForType(reflect.TypeOf(int(1)), nil)
	if err != nil || len(cache.Names) != 0 { t.Errorf("Unexpected state: got %v, %v; wanted %v, %v", err, cache, error(nil), fieldCacheEntry{}) }
	
	x0 := X0(x)
	v_ptr = reflect.ValueOf(&x0)
	v_raw = v_ptr.Elem()
	v_type = v_raw.Type()
	tagged_fields = 0
	
	cache, err = buildFieldCacheEntryForType(v_type, nil)
	if err != nil { t.Errorf("Unexpected return value (buildFieldCacheEntryForType()): got %v, expected nil", err) }
	if len(cache.Names) != tagged_fields { t.Errorf("Unexpected state (cache.Names): wrong length, got %d, expected %d", len(cache.Names), tagged_fields) }
	if len(cache.Fields) != tagged_fields { t.Errorf("Unexpected state (cache.Fields): wrong length, got %d, expected %d", len(cache.Fields), tagged_fields) }
	if len(cache.IsScanner) != tagged_fields { t.Errorf("Unexpected state (cache.IsScanner): wrong length, got %d, expected %d", len(cache.IsScanner), tagged_fields) }
}

type Y X1
type Z X1
func (z *Z) GetFields() (NamedFields, error) {
	return NamedFields{
		Names: []string{"testing1", "testing2"},
		Fields: []interface{}{&z.E2.Test, &z.private},
	}, nil
}

func Test_GetFieldsFrom(t *testing.T) {
	y := Y{E1: E1{Test: "value3"}, E2: E2{Test: "value4"}, Test1: "value1", Test2: "value2"}
	y_type := reflect.TypeOf(y)

	if cached, ok := fieldsCache[y_type]; ok { t.Errorf("Expected no cached value, but got one: %+v", cached) }
	_, err := GetFieldsFrom(y)
	if err == nil || !strings.Contains(err.Error(), "incompatible object type") { t.Errorf("Unexpected return value (buildNamedFieldsCacheForType()): got %v, expected 'not addressable' error", err) }
	if cached, ok := fieldsCache[y_type]; ok { t.Errorf("Expected no cached value, but got one: %+v", cached) }

	fields, err := GetFieldsFrom(&y)
	if err != nil { t.Errorf("Unexpected return value (GetFieldsFrom): got %v, expected nil", err) }
	if _, ok := fieldsCache[y_type]; !ok { t.Errorf("Expected cached value, but got empty value instead!") }
	fields_again, err := GetFieldsFrom(&y)
	if err != nil { t.Errorf("Unexpected return value (GetFieldsFrom): got %v, expected nil", err) }

	if !reflect.DeepEqual(fields, fields_again) { t.Errorf("Unexpected return values (GetFieldsFrom): %+v and %+v should be equal", fields, fields_again) }

	manually_constructed := NamedFields{
		Names: []string{"value3", "value3", "value4", "value1", "value2"},
		Fields: []interface{}{&y.E1.Test, &y.E2.E1.Test, &y.E2.Test, &y.Test1, &y.Test2},
	}
	if !reflect.DeepEqual(fields, manually_constructed) { t.Errorf("Unexpected return values (GetFieldsFrom): got %+v, expected %+v", fields, manually_constructed) }

	z := Z{E2: E2{Test: "testing1"}, private: "testing2"}
	fields, err = GetFieldsFrom(&z)
	if err != nil { t.Errorf("Unexpected return value (GetFieldsFrom): got %v, expected nil", err) }
	manually_constructed, err = z.GetFields()
	if !reflect.DeepEqual(fields, manually_constructed) { t.Errorf("Unexpected return values (GetFieldsFrom): %+v and %+v should be equal", fields, manually_constructed) }
}

type Y1 X1
type Z1 X1
func (z *Z1) GetFields() (NamedFields, error) {
	return NamedFields{
		Names: []string{"testing1", "testing2"},
		Fields: []interface{}{&z.E2.Test, &z.private},
	}, nil
}

func Test_BuildNamedFields(t *testing.T) {
	w := X0{}
	x := 10
	y := Y1{E1: E1{Test: "value3"}, E2: E2{Test: "value4"}, Test1: "value1", Test2: "value2"}
	z := Z1{E1: E1{Test: "testing1"}, private: "testing2"}

	fields_w, _ := w.GetFields()
	fields_y, _ := GetFieldsFrom(&y)
	fields_z, _ := GetFieldsFrom(&z)
	fields_yz, _ := GetFieldsFrom(&y)
	fields_yz.Append(fields_z)

	testcases := map[string]struct{
		args []ScanInto
		result NamedFields
		errmatch string
	}{
		"single":       {[]ScanInto{&y},      fields_y,      ""},
		"multi":        {[]ScanInto{&y, &z},  fields_yz,     ""},
		"non-struct":   {[]ScanInto{&x},      NamedFields{}, "incompatible object"},
		"non-struct-2": {[]ScanInto{&y, &x},  NamedFields{}, "incompatible object"},
		"non-struct-3": {[]ScanInto{&y, nil}, NamedFields{}, "incompatible object"},
		"empty":        {[]ScanInto{},        NamedFields{}, "empty output object list"},
		"no-default":   {[]ScanInto{&w},      fields_w,      ""},
	}

	for k, v := range testcases {
		t.Run(k, func (t *testing.T){
			fields, err := BuildNamedFields(v.args)
			if v.errmatch == "" && err != nil { t.Errorf("Unexpected return value (BuildNamedFields): got %v, expected nil", err) }
			if v.errmatch != "" && (err == nil || !strings.Contains(err.Error(), v.errmatch)) { t.Errorf("Unexpected return value (BuildNamedFields): got %v, expected '%s' error", err, v.errmatch) }

			if !reflect.DeepEqual(fields, v.result) { t.Errorf("Unexpected return values (GetFieldsFrom): %+v and %+v should be equal", fields, v.result) }
		})
	}
}

type P struct {
	err      error
	postScan bool
}

type Y2 P
type Z2 P
func (z *Z2) PostScan() error {
	z.postScan = true
	return z.err
}

func (z Z2) Get() bool { return z.postScan }
func (z Y2) Get() bool { return z.postScan }

func Test_postScan(t *testing.T) {
	abort := errors.New("abort!")
	testcases := map[string]struct{
		arg []ScanInto
		out error
		scanned []bool
	}{
		"noop":  {[]ScanInto{},                                                            nil, []bool{}},
		"simple": {[]ScanInto{&Y2{err: nil}, &Z2{err: nil}, &Y2{err: nil}, &Z2{err: nil}}, nil, []bool{false, true, false, true}},
		"error":  {[]ScanInto{&Z2{err: nil}, &Z2{err: abort}, &Z2{err: nil}},              abort, nil},
	}

	for k, v := range testcases {
		t.Run(k, func (t *testing.T){
			err := postScan(v.arg)
			if err != v.out { t.Errorf("Unexpected return values (postScan): got %v, expected %v", err, v.out) }
			if v.scanned != nil {
				for i := range v.scanned {
					if v.scanned[i] != v.arg[i].(interface{Get() bool}).Get() { t.Errorf("Unexpected state (P.postScan): got %v, expected %v", v.arg[i].(interface{Get() bool}).Get(), v.scanned[i]) }
				}
			}
		})
	}
}

type RowMock struct {
	columns []string
	values []string
	colerr error
}

func (r *RowMock) Scan(out ...interface{}) error {
	for i, v := range out {
		switch x := v.(type) {
		case sql.Scanner:
			x.Scan(r.values[i])
		case *string:
			*x = r.values[i]
		default:
			return fmt.Errorf("bad type: %v", v)
		}
	}

	return nil
}

func (r *RowMock) ColumnNames() ([]string, error) {
	return r.columns, r.colerr
}

type X2 struct {
	Field1 string `dml:"field_1"`
	Field2 string `dml:"field_2"`
	Field3 string `dml:"field_3"`
}

type X3 X2
func (x *X3) GetFields() (NamedFields, error) {
	return NamedFields{}, nil
}

type X4 X2
func (x *X4) GetFields() (NamedFields, error) {
	return NamedFields{}, errors.New("error when listing fields")
}

func Test_BuildMap(t *testing.T) {
	abort := errors.New("abort!")
	x := X2{"hello", "hi", "hey"}

	testcases := map[string]struct{
		rows *RowMock
		fields NamedFields
		output ScanMap
		err error
	}{
		"names-err": {
			&RowMock{
				columns: []string{"field_1", "field_2"},
				colerr: abort,
			},
			NamedFields{
				Names: []string{"field_1", "field_3"},
				Fields: []interface{}{&x.Field1, &x.Field3},
			},
			nil,
			abort,
		},
		"names-noop": {
			&RowMock{},
			NamedFields{
				Names: []string{"field_1", "field_3"},
				Fields: []interface{}{&x.Field1, &x.Field3},
			},
			nil,
			nil,
		},
		"names-basic": {
			&RowMock{
				columns: []string{"field_1", "field_2", "field_3"},
			},
			NamedFields{
				Names: []string{"field_3", "field_1", "field_2"},
				Fields: []interface{}{&x.Field3, &x.Field1, &x.Field2},
			},
			ScanMap{1, 2, 0},
			nil,
		},
		"names-huge": {
			&RowMock{
				columns: []string{"12", "13", "4", "10", "7", "1", "x", "14", "9", "3", "6", "y", "15", "11", "2", "5", "0", "8"},
			},
			NamedFields{
				Names: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "missing"},
				Fields: make([]interface{}, 16),
			},
			ScanMap{12,13,4,10,7,1,-1,14,9,3,6,-1,15,11,2,5,0,8},
			nil,
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			m, err := BuildMap(v.rows, v.fields)
			if err != v.err { t.Errorf("Unexpected return values (BuildMap): got %v, expected %v", err, v.err) }
			if !reflect.DeepEqual(m, v.output) { t.Errorf("Unexpected return values (BuildMap): got %v, expected %v", m, v.output) }
		})
	}
}

func Test_dumbAssFuckinAdapter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil { t.Skipf("couldn't create mock DB: %v", err) }

	defer db.Close()

	query := "SELECT field_1, field_2, field_3 FROM table"
	row_titles := []string{"field_1", "field_2", "field_3"}
	mock.ExpectQuery(query).WillReturnRows(
		sqlmock.NewRows([]string{"field_1", "field_2", "field_3"}).
		AddRow("row1", "row2", "row3"),
	)

	rows, _ := X(db.Query(query))
	names, err := rows.ColumnNames()
	if !reflect.DeepEqual(row_titles, names) { t.Errorf("Unexpected return values (ColumnNames): got %v, expected %v", names, row_titles) }
	if err != nil { t.Errorf("Unexpected return values (ColumnNames): got %v, expected nil", err) }
	rows.(dumbAssFuckinAdapter).sqlRows.(*sql.Rows).Close()
	names, err = rows.ColumnNames()
	if err == nil { t.Errorf("Unexpected return values (ColumnNames): got nil, expected anything else") }
}

func p(array []ScanInto) string {
	strs := make([]string, len(array))
	for i, o := range array {
		strs[i] = fmt.Sprintf("%+v", o)
	}
	return "[" + strings.Join(strs, ", ") + "]"
}

func Test_ScanWithMappedFields(t *testing.T) {
	var a X2

	testcases := map[string]struct{
		in, out []ScanInto
		row *RowMock
		smap ScanMap
		fields NamedFields
		err string
	}{
		"simple": {
			[]ScanInto{&a}, []ScanInto{&X2{"v3", "v1", ""}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			ScanMap{1, -1 ,0},
			NamedFields{Names: []string{"f1", "f2", "f3"}, Fields: []interface{}{&a.Field1, &a.Field2, &a.Field3}},
			"",
		},
		"empty": {
			[]ScanInto{}, []ScanInto{},
			&RowMock{},
			ScanMap{},
			NamedFields{},
			"empty",
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			err := ScanWithMappedFields(v.row, v.smap, v.fields)
			if v.err != "" && (err == nil || !strings.Contains(err.Error(), v.err)) { t.Errorf("Unexpected return values (ScanWithMappedFields): got unexpected error %v, looking for %s", err, v.err) }
			if v.err == "" && err != nil { t.Errorf("Unexpected return values (ScanWithMappedFields): got unexpected error %v, looking for nil", err) }
			if !reflect.DeepEqual(v.in, v.out) { t.Errorf("Unexpected operation result (ScanWithMappedFields): got %v, expected %v", p(v.in), p(v.out)) }
			for c, f := range v.smap {
				if !(f == -1 || v.row.values[c] == *v.fields.Fields[f].(*string)) { t.Errorf("Mismatched values: column %d (%s) != field %d (%s)", c, v.row.values[c], f, *v.fields.Fields[f].(*string)) }
				if !(f == -1 || v.row.columns[c] == v.fields.Names[f]) { t.Errorf("Mismatched names: column %d (%s) != field %d (%s)", c, v.row.columns[c], f, v.fields.Names[f]) }
			}
		})
	}
}

func Test_ScanWithMap(t *testing.T) {
	var a X2
	var b X3
	var c X4

	testcases := map[string]struct{
		in, out []ScanInto
		row *RowMock
		smap ScanMap
		err string
	}{
		"simple": {
			[]ScanInto{&a}, []ScanInto{&X2{"v3", "v1", ""}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			ScanMap{1, -1 ,0},
			"",
		},
		"empty": {
			[]ScanInto{&b}, []ScanInto{&X3{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			ScanMap{1, -1, 0},
			"empty list",
		},
		"empty-err": {
			[]ScanInto{&c}, []ScanInto{&X4{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			ScanMap{1, -1, 0},
			"listing fields",
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			err := ScanWithMap(v.row, v.smap, v.in...)
			if v.err != "" && (err == nil || !strings.Contains(err.Error(), v.err)) { t.Errorf("Unexpected return values (ScanWithMap): got unexpected error %v, looking for %s", err, v.err) }
			if v.err == "" && err != nil { t.Errorf("Unexpected return values (ScanWithMap): got unexpected error %v, looking for nil", err) }
			if !reflect.DeepEqual(v.in, v.out) { t.Errorf("Unexpected operation result (ScanWithMap): got %v, expected %v", p(v.in), p(v.out)) }
		})
	}
}

func Test_ScanWithFields(t *testing.T) {
	var a X2
	var b X3
	var c X4

	testcases := map[string]struct{
		in, out []ScanInto
		row *RowMock
		fields NamedFields
		err string
	}{
		"simple": {
			[]ScanInto{&a}, []ScanInto{&X2{"v3", "v1", ""}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			NamedFields{Names: []string{"f1", "f2", "f3"}, Fields: []interface{}{&a.Field1, &a.Field2, &a.Field3}},
			"",
		},
		"empty": {
			[]ScanInto{&b}, []ScanInto{&X3{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			NamedFields{Names: []string{}, Fields: []interface{}{}},
			"empty list",
		},
		"empty-error": {
			[]ScanInto{&c}, []ScanInto{&X4{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}, colerr: errors.New("abort")},
			NamedFields{Names: []string{}, Fields: []interface{}{}},
			"abort",
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			err := ScanWithFields(v.row, v.fields)
			if v.err != "" && (err == nil || !strings.Contains(err.Error(), v.err)) { t.Errorf("Unexpected return values (ScanWithFields): got unexpected error %v, looking for %s", err, v.err) }
			if v.err == "" && err != nil { t.Errorf("Unexpected return values (ScanWithFields): got unexpected error %v, looking for nil", err) }
			if !reflect.DeepEqual(v.in, v.out) { t.Errorf("Unexpected operation result (ScanWithFields): got %v, expected %v", p(v.in), p(v.out)) }
		})
	}
}

func Test_Scan(t *testing.T) {
	var a X2
	var b X3
	var c X4

	testcases := map[string]struct{
		in, out []ScanInto
		row *RowMock
		err string
	}{
		"simple": {
			[]ScanInto{&a}, []ScanInto{&X2{"v3", "v1", ""}},
			&RowMock{columns: []string{"field_2", "missing", "field_1"}, values: []string{"v1", "v2", "v3"}},
			"",
		},
		"empty": {
			[]ScanInto{&b}, []ScanInto{&X3{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			"empty",
		},
		"empty-error": {
			[]ScanInto{&c}, []ScanInto{&X4{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			"listing fields",
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			err := Scan(v.row, v.in...)
			if v.err != "" && (err == nil || !strings.Contains(err.Error(), v.err)) { t.Errorf("Unexpected return values (Scan): got unexpected error %v, looking for %s", err, v.err) }
			if v.err == "" && err != nil { t.Errorf("Unexpected return values (Scan): got unexpected error %v, looking for nil", err) }
			if !reflect.DeepEqual(v.in, v.out) { t.Errorf("Unexpected operation result (Scan): got %v, expected %v", p(v.in), p(v.out)) }
		})
	}
}

func Test_QuickScan(t *testing.T) {
	var a X2
	var b X3
	var c X4

	testcases := map[string]struct{
		in, out []ScanInto
		row *RowMock
		err string
	}{
		"simple": {
			[]ScanInto{&a}, []ScanInto{&X2{"v1", "v2", "v3"}},
			&RowMock{values: []string{"v1", "v2", "v3"}},
			"",
		},
		"empty": {
			[]ScanInto{&b}, []ScanInto{&X3{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			"empty",
		},
		"empty-error": {
			[]ScanInto{&c}, []ScanInto{&X4{}},
			&RowMock{columns: []string{"f2", "missing", "f1"}, values: []string{"v1", "v2", "v3"}},
			"listing fields",
		},
	}

	for k, v := range testcases {
		t.Run(k, func(t *testing.T) {
			err := QuickScan(v.row, v.in...)
			if v.err != "" && (err == nil || !strings.Contains(err.Error(), v.err)) { t.Errorf("Unexpected return values (Scan): got unexpected error %v, looking for %s", err, v.err) }
			if v.err == "" && err != nil { t.Errorf("Unexpected return values (Scan): got unexpected error %v, looking for nil", err) }
			if !reflect.DeepEqual(v.in, v.out) { t.Errorf("Unexpected operation result (Scan): got %v, expected %v", p(v.in), p(v.out)) }
		})
	}
}
