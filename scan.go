package dml

import (
	"errors"
)

// QuickScan does the most basic (but also the highest performance) guided scan.
// a nil map is used, so all of the fields in `into` are scanned into verbatim,
// and it is the caller's responsibility to ensure that the correct number of
// fields is present, as well as ensuring their order.
// If the fields are not exactly appropriate for the Scannable, or if `into`
// is empty, an error will be returned. Likewise, an error will be returned if
// there is an underlying error while performing the scan.
func QuickScan(s Scannable, into ...ScanInto) error {
	fields, err := BuildNamedFields(into)
	if err != nil { return err }
	if err = ScanWithMappedFields(s, nil, fields); err != nil { return err }
	return postScan(into)
}

// Scan does the full gamut of pre-processing and field matching.  It is designed
// to work well for objects which are large and complex and may need to be populated
// in a variety of different ways, from a variety of different queries. Fields in
// `into` will be dynamically mapped to available columns provided by `adv`. For 
// details about this process, see BuildMap. A ScanMap is automatically built to
// map columns in `adv` to fields in `into`, and a NamedFields is automatically
// built from `into`. Errors during the underlying scan are propagated to the caller.
func Scan(adv AdvancedScannable, into ...ScanInto) error {
	fields, err := BuildNamedFields(into)
	if err != nil { return err }
	if err = ScanWithFields(adv, fields); err != nil { return err }
	return postScan(into)
}

// ScanWithFields takes a pre-existing NamedFields. Otherwise it works the same way as Scan.
func ScanWithFields(adv AdvancedScannable, fields NamedFields) error {
	m, err := BuildMap(adv, fields)
	if err != nil { return err }
	return ScanWithMappedFields(adv, m, fields)
}

// ScanWithMap takes a pre-existing ScanMap. Otherwise it works the same way as Scan.
func ScanWithMap(s Scannable, m ScanMap, into ...ScanInto) error {
	fields, err := BuildNamedFields(into)
	if err != nil { return err }
	if err = ScanWithMappedFields(s, m, fields); err != nil { return err }
	return postScan(into)
}

// ScanWithMappedFields is the implementation upon which all others eventually land.
// It uses the provided ScanMap to reorder the field list so that the desired columns
// match the desired fields. Columns may map to zero or one fields. If a column does
// not map to a field, it will receive a no-op mapping and its value will be discarded.
// This function makes no attempt to check that the type of the field a column maps to
// is appropriate to receive values from that column, only that the names match; values
// with incompatible types being passed to Scannable.Scan will result in errors which will
// propagate up to the caller.
func ScanWithMappedFields(s Scannable, m ScanMap, fields NamedFields) error {
	field_list := fields.Fields
	if m != nil {
		field_list = make([]interface{}, len(m))
		for idx_from, idx_into := range m {
			if idx_into == -1 {
				field_list[idx_from] = noopScanner{}
			} else {
				field_list[idx_from] = fields.Fields[idx_into]
			}
		}
	}
	if len(field_list) == 0 { return errors.New("cannot scan into empty list of fields") }
	return s.Scan(field_list...)
}
