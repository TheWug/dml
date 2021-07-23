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

// AdvancedScannable is Scannable, plus a ColumnTypes function (as provided by *sql.Rows) which allows
// the caller to see how many columns are coming in and what their types and names are. Internally,
// this information is used to construct a mapping of scannable fields to destination object fields.
type AdvancedScannable interface {
	Scannable
	
	ColumnTypes() ([]*sql.ColumnType, error)
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
func (s *scannableWrapper) ColumnTypes() ([]*sql.ColumnType, error) {
	return nil, nil
}

// Used to wrap an *sql.Row or similar object which does not support the IterableScannable interface.
func WrapBasic(inner Scannable) IterableScannable {
	return &scannableWrapper{Scannable: inner}
}
