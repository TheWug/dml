package dml

// Any struct containing `dml` field tags is considered to be a ScanInto, and can be the subject of a Scan.
// structs which are processed by this package use field tags similarly to how the `json` package uses them.
// those tags are used to determine a mapping by which a structure's fields are populated by a Scannable.
//
// see the following example:
// type Example struct {                 // fields will be populated from Scannable columns:
//     Id        int `dml:"table_id"`    // table_id
//     TestField int `dml:"table_field"` // table_field
// }
//
// Remember that each field in the scannable output can only output its value once. However, multiple fields
// in the output may have the same name. Tagged fields, from top to bottom, will be mapped to fields
// appearing in the output, from left to right. For operations which process multiple objects, this allows
// you to marshal data from a single row into several objects of the same type.
type ScanInto interface{}

// Same deal as above, except this one expects a typed array.
type ScanIntoArray interface{}

// There also exists the following interface, which allows you to provide your own GetFields implementation
// for a struct.  You can use this to provide field wrappers, multi-level field population, and other more
// complex features in an explicit way.
type GetFields interface {
	GetFields() (NamedFields, error)
}

// Some structs may choose to store their field values in the database in some sort of intermediate format.
// those structs can implement ScanIntoPostProcessable, to receive an extra callback after the scan finishes,
// to assist with converting between formats.
type ScanIntoPostProcessable interface {
	PostScan() error
}

// nilScanner is an internal type which is used to discard fields from queries which are not requested.
type noopScanner struct {}

// nilScanner.Scan() simply discards its input.
func (n noopScanner) Scan(interface{}) error { return nil }