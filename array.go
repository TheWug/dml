package dml

import (
	"errors"
	"reflect"
)

// for any slice []T, returns the zero value of type T.
func zeroValueForSliceContents(slice reflect.Value) interface{} {
	return reflect.Zero(slice.Type().Elem()).Interface()
}

// for any array of pointers to slice *[]T, return an array of slices []T.
func getSlices(into []ScanIntoArray) (out []reflect.Value, err error) {
	for i := range into {
		ps := reflect.ValueOf(i)
		if ps.Kind() != reflect.Ptr {
			return nil, errors.New("elements must be a pointers to slices")
		}
		ps = ps.Elem()
		if ps.Kind() != reflect.Slice {
			return nil, errors.New("argument must be a pointer to slice")
		}
		out = append(out, ps)
	}

	return out, nil
}

func renderInto(slices []reflect.Value) (out []reflect.Value) {
	for _, s := range slices {
		out = append(out, s.Index(s.Len() - 1))
	}

	return out
}

func ScanArray(it IterableScannable, into ...ScanIntoArray) error {
	slices, err := getSlices(into)
	if err != nil { return err }

	zeros := make([]ScanInto, len(slices))
	for i := range slices {
		zeros[i] = zeroValueForSliceContents(slices[i])
	}

	values, types, err := internalNormalizeObjects(zeros, true)
	if err != nil { return err }

	nfm, err := GetNamedFieldsMakers(types)
	if err != nil { return err }

	named_fields, err := RenderNamedFields(nfm, values)
	if err != nil { return err }

	smap, err := BuildMap(it, named_fields)
	if err != nil { return err }

	rewind := true
	defer func() {
		if !rewind { return }
		for i := range slices {
			slices[i].SetLen(slices[i].Len() - 1)
		}
	}()

	for it.Next() {
		// first, append the zero value to each array
		for i, s := range slices {
			s.Set(reflect.Append(s, reflect.ValueOf(zeros[i])))
		}

		if err := it.Err(); err != nil { return err }

		named_fields, err = RenderNamedFields(nfm, renderInto(slices))
		if err != nil { return err }

		// now try to scan
		err = ScanWithMappedFields(it, smap, named_fields)
		if err != nil { return err }
	}

	rewind = false
	return nil
}
