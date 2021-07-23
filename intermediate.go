package dml

import (
	"errors"
)

// this represents a simple index based mapping from expected final position (in Scan call)
// to source position (in Fields list).  If the source position has the special value -1,
// it is considered to be a no-op, and scans into nothing.
// When the same query reads data into the same type of object, the ScanMap can be re-used
// for a performance improvement.
type ScanMap []int

// iln is a helper struct for BuildMap. It's a linked list where the first node maintains
// a pointer to the last node for convenience of appending.
type iln struct {
	index int
	next, end *iln
}

// appends an index to the end of the list. returns the new list head
func (n *iln) add(i int) *iln {
	if n == nil {
		x := &iln{index: i}
		x.end = x
		return x
	} else {
		n.end.next = &iln{index: i}
		n.end = n.end.next
		return n
	}
}

// pulls an index from the beginning of the list. returns the new list head.
// returns nil if the list is empty.
func (n *iln) yoink() (*int, *iln) {
	if n == nil {
		return nil, nil
	} else {
		if n.end != n { n.next.end = n.end }
		return &n.index, n.next
	}
}

// BuildMap builds a ScanMap from the provided scannable and field list.
func BuildMap(adv AdvancedScannable, fields NamedFields) (ScanMap, error) {
	columns, err := adv.ColumnTypes()
	if err != nil { return nil, err }
	
	// special case: if columns is nil, that probably means adv is a scannableWrapper,
	// so we want to return the special output value nil to indicate "skip the mapping step".
	if columns == nil { return nil, nil }
	
	output := make(ScanMap, len(columns))
	for i := range columns { output[i] = -1 }
	
	if (len(columns) - 5) * (len(fields.Names) - 5) > 100 {
		columnsByName := make(map[string]*iln)
		for i, c := range columns { columnsByName[c.Name()] = columnsByName[c.Name()].add(i) }
		for i, n := range fields.Names {
			var x *int
			x, columnsByName[n] = columnsByName[n].yoink()
			if x != nil { output[*x] = i }
		}
	} else {
		MainLoop:
		for i, n := range fields.Names {
			for j, c := range columns {
				if output[j] != -1 { continue }
				if n == c.Name() {
					output[j] = i
					continue MainLoop
				}
			}
			
			// if we get here, that means a field is requesting a column which doesn't exist.
			// that's okay, that field just won't be populated.
		}
	}
	
	return output, nil
}

// NamedFields represents a list of fields and their associated names, and is used to match
// fields to columns in the output database.
type NamedFields struct {
	Names  []string
	Fields []interface{}
}

// n.Append(other) appends NamedFields `other` object `n`.
func (n *NamedFields) Append(other NamedFields) {
	n.Names  = append(n.Names,  other.Names...)
	n.Fields = append(n.Fields, other.Fields...)
}

// n.Push(name, field) adds a new field `field`, named `name`.
func (n *NamedFields) Push(name string, field interface{}) {
	n.Names  = append(n.Names,  name)
	n.Fields = append(n.Fields, field)
}
// BuildNamedFields builds a NamedFields from the provided list of ScanInto objects.
func BuildNamedFields(into []ScanInto) (NamedFields, error) {
	if len(into) == 0 { return NamedFields{}, errors.New("empty output object list") }
	
	fields, err := GetFieldsFrom(into[0])
	if err != nil { return NamedFields{}, err }
	for _, x := range into[1:] {
		new_fields, new_err := GetFieldsFrom(x)
		if new_err != nil { return NamedFields{}, new_err }
		fields.Append(new_fields)
	}
	
	return fields, nil
}

func postScan(into []ScanInto) error {
	for _, i := range into {
		if pp, ok := i.(ScanIntoPostProcessable); ok {
			if err := pp.PostScan(); err != nil { return err }
		}
	}
	
	return nil
}
