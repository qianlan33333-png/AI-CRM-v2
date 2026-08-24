package main

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

type cloneVisit struct {
	typeOf  reflect.Type
	pointer uintptr
}

func cloneOpenAPIDocument(source *openapi3.T) *openapi3.T {
	if source == nil {
		return nil
	}
	return cloneOpenAPIValue(reflect.ValueOf(source), make(map[cloneVisit]reflect.Value)).Interface().(*openapi3.T)
}

func cloneOpenAPIValue(source reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if !source.IsValid() {
		return reflect.Value{}
	}

	switch source.Kind() {
	case reflect.Interface:
		if source.IsNil() {
			return reflect.Zero(source.Type())
		}
		clone := reflect.New(source.Type()).Elem()
		clone.Set(cloneOpenAPIValue(source.Elem(), seen))
		return clone
	case reflect.Pointer:
		if source.IsNil() {
			return reflect.Zero(source.Type())
		}
		visit := cloneVisit{typeOf: source.Type(), pointer: source.Pointer()}
		if clone, ok := seen[visit]; ok {
			return clone
		}

		switch source := source.Interface().(type) {
		case *openapi3.Paths:
			clone := openapi3.NewPathsWithCapacity(source.Len())
			result := reflect.ValueOf(clone)
			seen[visit] = result
			clone.Extensions = cloneOpenAPIValue(reflect.ValueOf(source.Extensions), seen).Interface().(map[string]any)
			clone.Origin = cloneOpenAPIValue(reflect.ValueOf(source.Origin), seen).Interface().(*openapi3.Origin)
			for name, item := range source.Map() {
				clone.Set(name, cloneOpenAPIValue(reflect.ValueOf(item), seen).Interface().(*openapi3.PathItem))
			}
			return result
		case *openapi3.Responses:
			clone := openapi3.NewResponsesWithCapacity(source.Len())
			result := reflect.ValueOf(clone)
			seen[visit] = result
			clone.Extensions = cloneOpenAPIValue(reflect.ValueOf(source.Extensions), seen).Interface().(map[string]any)
			clone.Origin = cloneOpenAPIValue(reflect.ValueOf(source.Origin), seen).Interface().(*openapi3.Origin)
			for status, response := range source.Map() {
				clone.Set(status, cloneOpenAPIValue(reflect.ValueOf(response), seen).Interface().(*openapi3.ResponseRef))
			}
			return result
		case *openapi3.Callback:
			clone := openapi3.NewCallbackWithCapacity(source.Len())
			result := reflect.ValueOf(clone)
			seen[visit] = result
			clone.Extensions = cloneOpenAPIValue(reflect.ValueOf(source.Extensions), seen).Interface().(map[string]any)
			clone.Origin = cloneOpenAPIValue(reflect.ValueOf(source.Origin), seen).Interface().(*openapi3.Origin)
			for expression, item := range source.Map() {
				clone.Set(expression, cloneOpenAPIValue(reflect.ValueOf(item), seen).Interface().(*openapi3.PathItem))
			}
			return result
		}

		clone := reflect.New(source.Type().Elem())
		seen[visit] = clone
		clone.Elem().Set(cloneOpenAPIValue(source.Elem(), seen))
		return clone
	case reflect.Struct:
		if source.Type().PkgPath() != "github.com/getkin/kin-openapi/openapi3" {
			return source
		}
		clone := reflect.New(source.Type()).Elem()
		for field := range source.NumField() {
			if source.Type().Field(field).IsExported() {
				clone.Field(field).Set(cloneOpenAPIValue(source.Field(field), seen))
			}
		}
		return clone
	case reflect.Map:
		if source.IsNil() {
			return reflect.Zero(source.Type())
		}
		clone := reflect.MakeMapWithSize(source.Type(), source.Len())
		iterator := source.MapRange()
		for iterator.Next() {
			clone.SetMapIndex(iterator.Key(), cloneOpenAPIValue(iterator.Value(), seen))
		}
		return clone
	case reflect.Slice:
		if source.IsNil() {
			return reflect.Zero(source.Type())
		}
		clone := reflect.MakeSlice(source.Type(), source.Len(), source.Len())
		for index := range source.Len() {
			clone.Index(index).Set(cloneOpenAPIValue(source.Index(index), seen))
		}
		return clone
	case reflect.Array:
		clone := reflect.New(source.Type()).Elem()
		for index := range source.Len() {
			clone.Index(index).Set(cloneOpenAPIValue(source.Index(index), seen))
		}
		return clone
	default:
		return source
	}
}

func TestCloneOpenAPIDocumentIsIndependent(t *testing.T) {
	baseline, _, err := load(specPath, mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	first := cloneOpenAPIDocument(baseline)
	second := cloneOpenAPIDocument(baseline)

	first.Paths.Value("/healthz").Get.OperationID = "mutated"
	first.Paths.Value("/healthz").Get.Responses.Set("599", &openapi3.ResponseRef{Value: openapi3.NewResponse()})
	first.Components.Schemas["HealthResponse"].Value.Required = append(
		first.Components.Schemas["HealthResponse"].Value.Required,
		"mutated",
	)

	if got := second.Paths.Value("/healthz").Get.OperationID; got != "getHealthz" {
		t.Fatalf("path mutation leaked into independent clone: %q", got)
	}
	if response := second.Paths.Value("/healthz").Get.Responses.Value("599"); response != nil {
		t.Fatal("response-map mutation leaked into independent clone")
	}
	for _, field := range second.Components.Schemas["HealthResponse"].Value.Required {
		if field == "mutated" {
			t.Fatal("schema mutation leaked into independent clone")
		}
	}
}

func TestCloneOpenAPIDocumentPreservesMapLikeCollectionsAndCycles(t *testing.T) {
	cyclicSchema := openapi3.NewObjectSchema()
	cyclicRef := &openapi3.SchemaRef{Ref: "#/components/schemas/Cyclic", Value: cyclicSchema}
	cyclicSchema.Properties = openapi3.Schemas{"self": cyclicRef}
	callback := openapi3.NewCallback(openapi3.WithCallback(
		"{$request.body#/callback}",
		&openapi3.PathItem{Post: &openapi3.Operation{
			OperationID: "callback",
			Responses:   openapi3.NewResponsesWithCapacity(0),
		}},
	))
	source := &openapi3.T{
		Components: &openapi3.Components{
			Schemas:   openapi3.Schemas{"Cyclic": cyclicRef},
			Callbacks: openapi3.Callbacks{"Callback": {Value: callback}},
		},
		Paths: openapi3.NewPaths(),
	}

	clone := cloneOpenAPIDocument(source)
	clonedSchema := clone.Components.Schemas["Cyclic"].Value
	if clonedSchema == cyclicSchema {
		t.Fatal("schema pointer is shared with baseline")
	}
	if clonedSchema.Properties["self"].Value != clonedSchema {
		t.Fatal("cyclic schema reference was not preserved")
	}
	clonedCallback := clone.Components.Callbacks["Callback"].Value
	item := clonedCallback.Value("{$request.body#/callback}")
	if item == nil || item.Post == nil || item.Post.OperationID != "callback" {
		t.Fatal("callback map was not preserved")
	}
	item.Post.OperationID = "mutated"
	if got := callback.Value("{$request.body#/callback}").Post.OperationID; got != "callback" {
		t.Fatalf("callback mutation leaked into baseline: %q", got)
	}
}

func BenchmarkCloneOpenAPIDocument(b *testing.B) {
	doc, _, err := load(specPath, mappingPath)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		cloneOpenAPIDocument(doc)
	}
}
