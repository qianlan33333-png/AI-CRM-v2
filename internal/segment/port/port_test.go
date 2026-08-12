package port

import (
	"reflect"
	"sort"
	"testing"
)

func TestSegmentServiceSurfaceIsFrozen(t *testing.T) {
	typeOf := reflect.TypeOf((*Service)(nil)).Elem()
	methods := make([]string, typeOf.NumMethod())
	for index := 0; index < typeOf.NumMethod(); index++ {
		methods[index] = typeOf.Method(index).Name
	}
	sort.Strings(methods)
	want := []string{"Create", "Get", "List", "ListMembers", "RequestRefresh", "Update"}
	if !reflect.DeepEqual(methods, want) {
		t.Fatalf("methods = %v, want %v", methods, want)
	}
}
