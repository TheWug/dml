package dml

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// sqlScannerType is a helper variable for buildFieldCacheEntryForType.

var sqlScannerType = reflect.TypeOf((*sql.Scanner)(nil)).Elem()
var getFieldsType = reflect.TypeOf((*GetFields)(nil)).Elem()

// GetFieldsFrom populates fieldsCache with an appropriate entry if necessary, and then uses the cached value
// to build a suitable NamedFields object for the given input.
func GetFieldsFrom(into ...ScanInto) (output NamedFields, err error) {
	values, types, err := NormalizeObjects(into)
	if err != nil { return NamedFields{}, err }

	nfm, err := GetNamedFieldsMakers(types)
	if err != nil { return NamedFields{}, err }

	return RenderNamedFields(nfm, values)
}

// this function takes an array of generic interfaces and explores them, searching for suitable
// types and values for the rest of this library to use, and returning a list of values and types,
// plus an error describing what (if anything) went wrong. If an error is raised, the other return
// values will be nil.
func NormalizeObjects(into []ScanInto) (out_vals []reflect.Value, out_types []reflect.Type, err error) {
	return internalNormalizeObjects(into, false)
}

func internalNormalizeObjects(into []ScanInto, ignoreUnsettable bool) (out_vals []reflect.Value, out_types []reflect.Type, err error) {
	Outer:
	for _, i := range into {
		// if it's already a reflect.Value just recover its original value with a cast.
		// otherwise, make it into one.
		v, ok := i.(reflect.Value)
		if !ok {
			v = reflect.ValueOf(i)
		}

		for v.Kind() != reflect.Invalid  {
			if v.Type().Implements(getFieldsType) {
				out_vals = append(out_vals, v)
				out_types = append(out_types, v.Type())
				continue Outer
			}

			if v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
				v = v.Elem()
				continue
			}

			break
		}

		// if the value doesn't implement sql.Scanner, that means we must examine/modify
		// its fields directly ourself, and therefore it must be a settable struct
		if (ignoreUnsettable || v.CanSet()) && v.Kind() == reflect.Struct {
			out_vals = append(out_vals, v)
			out_types = append(out_types, v.Type())
			continue Outer
		}

		return nil, nil, fmt.Errorf("incompatible object type: %v (need a pointer or an sql.Scanner)", v.Kind())
	}

	return out_vals, out_types, nil
}

// Take the types of several objects and return an array of objects capable of marshalling
// each of them into NamedFields objects.
func GetNamedFieldsMakers(types []reflect.Type) (output []NamedFieldsMaker, err error) {
	for _, t := range types {
		cached, err := getFieldCachesFor(t)
		if err != nil { return nil, err }
		output = append(output, cached)
	}

	return output, nil
}

func RenderNamedFields(nfm []NamedFieldsMaker, values []reflect.Value) (output NamedFields, err error) {
	if len(nfm) != len(values) { return NamedFields{}, errors.New("incorrect number of parameters") }
	for i, n := range nfm {
		fields, err := n.NamedFields(values[i])
		if err != nil { return NamedFields{}, err }
		output.Append(fields)
	}

	return output, nil
}

// buildFieldCacheEntryForType takes a reflect.Type with kind == struct, and parses the struct
// definition, looking for `dml` field tags and using them to construct an instance-agnostic
// roadmap of the struct's fields which can later be used to efficiently build a NamedFields
// object for an instance of that type.
//
// Anonymous nested structs are traversed into as well (named ones are not, as a row from
// an SQL query is an inherently one dimensional structure). Unexported fields are ignored.
func buildFieldCacheEntryForType(t reflect.Type, path []int) (output fieldCacheEntry, err error) {
	defer func() { if r := recover(); r != nil { err = fmt.Errorf("%v", r) } }()
	if t.Kind() != reflect.Struct { return fieldCacheEntry{}, errors.New("tried to analyze field structure of non-struct type") }

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if db_field, ok := field.Tag.Lookup("dml"); len(field.PkgPath) == 0 && ok {
			output.Push(db_field, path, i, field.Type.Implements(sqlScannerType))
		} else if field.Anonymous && field.Type.Kind() == reflect.Struct {
			sub_cache, sub_error := buildFieldCacheEntryForType(field.Type, append(path, i))
			if sub_error != nil { return fieldCacheEntry{}, fmt.Errorf("error examining field %s: %w", field.Name, sub_error) }
			output.Append(sub_cache)
		}
	}

	return output, nil
}

// getCachedFieldsFor fetches a NamedFieldsMaker for this type, which is either a cached representation
// of the relevant fields of the type, or a passthru shim which handles GetFields implementors.
func getFieldCachesFor(t reflect.Type) (output NamedFieldsMaker, err error) {
	// if i implements GetFields, call its GetFields function instead of doing a manual examination.

	if t.Implements(getFieldsType){
		return namedFieldsFromGetFields{}, nil
	}

	// unwrap pointer/interface indirections
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Interface {
		t = t.Elem()
	}

	// we must get to an addressable struct. also catch nil pointers, where v.Kind() == reflect.Invalid
	if t.Kind() != reflect.Struct {
		return fieldCacheEntry{}, errors.New("nested object is not struct")
	}

	// lookup from, and if necessary populate, fieldsCache for this type
	fieldsCacheLock.RLock()
	cached, ok := fieldsCache[t]
	fieldsCacheLock.RUnlock()
	if !ok {
		fieldsCacheLock.Lock()
		cached, ok = fieldsCache[t]
		if !ok {
			cached, err = buildFieldCacheEntryForType(t, nil)
			if err != nil { return fieldCacheEntry{}, err }
			fieldsCache[t] = cached
		}
		fieldsCacheLock.Unlock()
	}

	return cached, nil
}

// NamedFieldsMaker provides a consistent interface for storing the cached field info about a type.
type NamedFieldsMaker interface {
	NamedFields(v reflect.Value) (n NamedFields, err error)
}

// namedFieldsFromGetFields is a thin shim which facilitates building a NamedFields from a GetFields implementor.
type namedFieldsFromGetFields struct {}

// NamedFields passes GetFields to its argument and returns the result. Its argument should implement GetFields.
func (n namedFieldsFromGetFields) NamedFields(v reflect.Value) (NamedFields, error) {
	if g, ok := v.Interface().(GetFields); ok && g != nil {
		return g.GetFields()
	}

	// This should be impossible to reach.
	return NamedFields{}, errors.New("Object does not implement GetFields")
}

// fieldCacheEntry is an internal type representing an instance-agnostic set of fields.
type fieldCacheEntry struct {
	Names []string
	Fields [][]int
	IsScanner []bool
}

// fieldCacheEntry.Push adds a new field into a fieldCacheEntry.
func (c *fieldCacheEntry) Push(name string, prefix []int, value int, scanner bool) *fieldCacheEntry {
	c.Names = append(c.Names, name)
	c.Fields = append(c.Fields, append(prefix[:len(prefix):len(prefix)], value))
	c.IsScanner = append(c.IsScanner, scanner)
	return c
}

// fieldCacheEntry.Append adds all of the fields in one fieldCacheEntry onto another.
func (c *fieldCacheEntry) Append(other fieldCacheEntry) *fieldCacheEntry {
	c.Names = append(c.Names, other.Names...)
	c.Fields = append(c.Fields, other.Fields...)
	c.IsScanner = append(c.IsScanner, other.IsScanner...)
	return c
}

// fieldCacheEntry.NamedFields renders a NamedFields object for the provided instance.
// normal operation should never produce an error. Errors can happen only if v does not match
// the type for which this fieldCacheEntry was generated, most likely due to tampering.
func (c fieldCacheEntry) NamedFields(v reflect.Value) (n NamedFields, err error) {
	n = NamedFields{
		Names: make([]string, 0, len(c.Names)),
		Fields: make([]interface{}, 0, len(c.Names)),
	}

	for i := range c.Names {
		f := v.FieldByIndex(c.Fields[i])
		if !c.IsScanner[i] {
			f = f.Addr()
		}
		n.Push(c.Names[i], f.Interface())
	}

	return n, nil
}

// fieldCaches is an internal cache of field representations, optimized for rendering to NamedFields objects.
var fieldsCache = make(map[reflect.Type]fieldCacheEntry)

// fieldsCacheLock is a mutex which protects fieldsCache from concurrent read/write.
var fieldsCacheLock sync.RWMutex
