package tree

import (
	"fmt"
	"reflect"
)

type TreeVisitor interface {
	GetDisplayName(obj interface{}) string
	GetChildren(obj interface{}) []interface{}
	ShouldSkip(obj interface{}) bool
}

type DefaultTreeVisitor struct {
	MaxDepth  int
	SkipEmpty bool
	ShowTypes bool
}

func (v *DefaultTreeVisitor) GetDisplayName(obj interface{}) string {
	if obj == nil {
		return "<nil>"
	}

	value := reflect.ValueOf(obj)
	typ := reflect.TypeOf(obj)

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return "<nil>"
		}
		value = value.Elem()
		typ = typ.Elem()
	}

	var displayName string

	switch value.Kind() {
	case reflect.Struct:
		displayName = typ.Name()
		if displayName == "" {
			displayName = "struct"
		}
	case reflect.Slice, reflect.Array:
		displayName = fmt.Sprintf("%s[%d]", typ.Elem().Name(), value.Len())
	case reflect.Map:
		displayName = fmt.Sprintf("map[%s]%s (%d items)", typ.Key().Name(), typ.Elem().Name(), value.Len())
	default:
		displayName = fmt.Sprintf("%v", value.Interface())
	}

	if v.ShowTypes {
		displayName = fmt.Sprintf("%s (%s)", displayName, typ.String())
	}

	return displayName
}

func (v *DefaultTreeVisitor) GetChildren(obj interface{}) []interface{} {
	if obj == nil {
		return nil
	}

	value := reflect.ValueOf(obj)

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	var children []interface{}

	switch value.Kind() {
	case reflect.Struct:
		typ := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			fieldType := typ.Field(i)

			if !field.CanInterface() {
				continue
			}

			fieldInfo := map[string]interface{}{
				"name":  fieldType.Name,
				"value": field.Interface(),
				"tag":   string(fieldType.Tag),
			}
			children = append(children, fieldInfo)
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			indexInfo := map[string]interface{}{
				"name":  fmt.Sprintf("[%d]", i),
				"value": value.Index(i).Interface(),
			}
			children = append(children, indexInfo)
		}

	case reflect.Map:
		for _, key := range value.MapKeys() {
			mapValue := value.MapIndex(key)
			keyInfo := map[string]interface{}{
				"name":  fmt.Sprintf("[%v]", key.Interface()),
				"value": mapValue.Interface(),
			}
			children = append(children, keyInfo)
		}
	}

	return children
}

func (v *DefaultTreeVisitor) ShouldSkip(obj interface{}) bool {
	if !v.SkipEmpty {
		return false
	}

	if obj == nil {
		return true
	}

	value := reflect.ValueOf(obj)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return true
	}

	return false
}
