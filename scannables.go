package dml

import (
	"database/sql"
)

// Scannable is designed to represent a database row yielding object, such as *sql.Row or *sql.Rows.
// Only the scan operation is supported, and functions which accept this interface are expected, in
// essence, to perform a scan blindly, or guided by information outside of the scannable object.
type Scannable interface {
	Scan(...interface{}) error
}

// an interface which describes sql.Rows
type sqlRows interface {
	Scan(...interface{}) error
	ColumnTypes() ([]*sql.ColumnType, error)
	Next() bool
	Err() error
}

// sql.Rows.ColumnTypes() is stupid and unmockable, so fuck it. Stupid shim.
type dumbAssFuckinAdapter struct {
	sqlRows
}

// Shim ColumnNames() from ColumnTypes().
func (piss dumbAssFuckinAdapter) ColumnNames() ([]string, error) {
	columns, err := piss.sqlRows.ColumnTypes()
	if err != nil { return nil, err }
	names := []string{}
	for _, c := range columns {
		names = append(names, c.Name())
	}
	return names, nil
}

// Wrap an sql.Rows (or similar) in an adapter which converts ColumnTypes() to ColumnNames().
// usage: rows, err := X(tx.Query(...))
func X(rows sqlRows, err error) (IterableScannable, error) {
	return dumbAssFuckinAdapter{sqlRows: rows}, err
}

// AdvancedScannable is Scannable, plus a ColumnTypes function (as provided by *sql.Rows) which allows
// the caller to see how many columns are coming in and what their types and names are. Internally,
// this information is used to construct a mapping of scannable fields to destination object fields.
type AdvancedScannable interface {
	Scannable
	
	ColumnNames() ([]string, error)
}

// IterableScannable is AdvancedScannable, plus Next and Err. With these additional methods, you can
// iterate over the datasource, fetching values until end of input (or error) and assemble them into
// an aggregate structure of some sort.
type IterableScannable interface {
	AdvancedScannable
	
	Next() bool
	Err() error
}

// scannableWrapper is a shim used to make *sql.Row universally compatible with these functions. The
// shim expects to be used in a manner compliant with an *sql.Rows.
type scannableWrapper struct {
	Scannable
	done bool
}

// scannableWrapper.Next() returns true on the first call, and false on subsequent calls, indicating an array of length 1.
func (s *scannableWrapper) Next() bool {
	if s.done {
		return false
	} else {
		s.done = true
		return true
	}
}

// scannableWrapper.Err() always returns nil, reflecting sql.Row's philosophy of "raise all errors at scan time"
func (s *scannableWrapper) Err() error {
	return nil
}

// scannableWrapper.ColumnTypes() returns nil and no error. Callers should interpret this to mean
// no column name checks can be done, so just blindly pass whatever you have to scan.
func (s *scannableWrapper) ColumnNames() ([]string, error) {
	return nil, nil
}

// Used to wrap an *sql.Row or similar object which does not support the IterableScannable interface.
func WrapBasic(inner Scannable) IterableScannable {
	return &scannableWrapper{Scannable: inner}
}
