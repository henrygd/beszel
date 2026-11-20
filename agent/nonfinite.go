package agent

import (
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/henrygd/beszel/internal/entities/system"
)

// dropNonFinite replaces NaN and +/-Inf in the outgoing stats and info with 0.
// The hub stores each record's stats in a PocketBase JSON field; encoding/json
// rejects NaN and Inf, and PocketBase then stores the whole field as null, so a
// single bad float would blank every metric for the system. Each change in the
// set of replaced fields is logged once at WARN so the source can be found.
func (a *Agent) dropNonFinite(data *system.CombinedData) {
	var fixed []string
	replaceNonFinite(reflect.ValueOf(&data.Stats).Elem(), "stats", &fixed)
	replaceNonFinite(reflect.ValueOf(&data.Info).Elem(), "info", &fixed)
	sort.Strings(fixed)
	key := strings.Join(fixed, ",")
	if key != a.lastNonFinite {
		if key != "" {
			slog.Warn("Replaced non-finite values with 0", "fields", fixed)
		}
		a.lastNonFinite = key
	}
}

// replaceNonFinite walks v and zeroes every settable non-finite float,
// appending the dotted field path of each one it changes to fixed.
func replaceNonFinite(v reflect.Value, path string, fixed *[]string) {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); (math.IsNaN(f) || math.IsInf(f, 0)) && v.CanSet() {
			v.SetFloat(0)
			*fixed = append(*fixed, path)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			replaceNonFinite(v.Elem(), path, fixed)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if t.Field(i).IsExported() {
				replaceNonFinite(v.Field(i), path+"."+t.Field(i).Name, fixed)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			replaceNonFinite(v.Index(i), fmt.Sprintf("%s[%d]", path, i), fixed)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			// Map values are not addressable: fix a copy and store it back.
			elem := reflect.New(iter.Value().Type()).Elem()
			elem.Set(iter.Value())
			before := len(*fixed)
			replaceNonFinite(elem, fmt.Sprintf("%s[%v]", path, iter.Key().Interface()), fixed)
			if len(*fixed) > before {
				v.SetMapIndex(iter.Key(), elem)
			}
		}
	}
}
