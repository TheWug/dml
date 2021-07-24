package dml

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// namedFieldsCache is an internal type representing an instance-agnostic set of fields.
type namedFieldsCache struct {
	Names []string
	Fields [][]int
	IsScanner []bool
}

// namedFieldsCache.Push adds a new field into a namedFieldsCache.
func (c *namedFieldsCache) Push(name string, prefix []int, value int, scanner bool) *namedFieldsCache {
	c.Names = append(c.Names, name)
	c.Fields = append(c.Fields, append(prefix[:len(prefix):len(prefix)], value))
	c.IsScanner = append(c.IsScanner, scanner)
	return c
}

// namedFieldsCache.Append adds all of the fields in one namedFieldsCache onto another.
func (c *namedFieldsCache) Append(other namedFieldsCache) *namedFieldsCache {
	c.Names = append(c.Names, other.Names...)
	c.Fields = append(c.Fields, other.Fields...)
	c.IsScanner = append(c.IsScanner, other.IsScanner...)
	return c
}

// namedFieldsCache.NamedFields renders a NamedFields object for the provided instance.
// normal operation should never produce an error. Errors can happen only if v does not match
// the type for which this namedFieldsCache was generated, most likely due to tampering.
func (c *namedFieldsCache) NamedFields(v reflect.Value) (n NamedFields, err error) {
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
var fieldsCache = make(map[reflect.Type]namedFieldsCache)

// fieldsCacheLock is a mutex which protects fieldsCache from concurrent read/write.
var fieldsCacheLock sync.RWMutex

// GetFields populates fieldsCache with an appropriate entry if necessary, and then uses the cached value
// to build a suitable NamedFields object for the given input.
func GetFieldsFrom(i ScanInto) (output NamedFields, err error) {
	var v reflect.Value

	// special cases:
	// if i is a reflect.Value, use it verbatim.
	// if not, convert whatever i is into a reflect.Value.
	v, ok := i.(reflect.Value)
	if !ok {
		v = reflect.ValueOf(i)
	}
	
	// finally, if i implements GetFields, call its GetFields function instead of doing a manual examination.
	if g, ok := v.Interface().(GetFields); ok && g != nil {
		return g.GetFields()
	}
	
	// unwrap pointer/interface indirections
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	
	// we must get to an addressable struct. also catch nil pointers, where v.Kind() == reflect.Invalid
	if !v.CanAddr() || v.Kind() != reflect.Struct {
		return NamedFields{}, errors.New("nested object is not addressable struct")
	}
	
	// lookup from, and if necessary populate, fieldsCache for this type
	t := v.Type()
	fieldsCacheLock.RLock()
	cached, ok := fieldsCache[t]
	fieldsCacheLock.RUnlock()
	if !ok {
		fieldsCacheLock.Lock()
		cached, ok = fieldsCache[t]
		if !ok {
			cached, err = buildNamedFieldsCacheForType(t, nil)
			if err != nil { return NamedFields{}, err }
			fieldsCache[t] = cached
		}
		fieldsCacheLock.Unlock()
	}
	
	return cached.NamedFields(v)
}

// sqlScannerType is a helper variable for buildNamedFieldsCacheForType.
var sqlScannerType = reflect.TypeOf((*sql.Scanner)(nil)).Elem()

// buildNamedFieldsCacheForType automatically reads information from the provided object, using field tags
// in the struct definition to infer the desired field names. The structure is traversed flatly
// looking for eligible fields (that is to say, sub-structs are not traversed into, unless they are
// anonymous). Self referential structures may cause an infinite traversal and should be avoided.
// buildFieldsCacheForType returns an object-agnostic representation which can be used to quickly
// compute the actual NamedFields list for any given instance of the provided type.
func buildNamedFieldsCacheForType(t reflect.Type, path []int) (output namedFieldsCache, err error) {
	if t.Kind() != reflect.Struct { return namedFieldsCache{}, errors.New("tried to analyze field structure of non-struct type") }
	
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if db_field, ok := field.Tag.Lookup("dml"); len(field.PkgPath) == 0 && ok {
			output.Push(db_field, path, i, field.Type.Implements(sqlScannerType))
		} else if field.Anonymous && field.Type.Kind() == reflect.Struct {
			sub_cache, sub_error := buildNamedFieldsCacheForType(field.Type, append(path, i))
			if sub_error != nil { return namedFieldsCache{}, fmt.Errorf("error examining field %s: %w", field.Name, sub_error) }
			output.Append(sub_cache)
		}
	}
	
	return output, nil
}
