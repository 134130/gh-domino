package tree

import (
	"fmt"
	"reflect"

	"github.com/charmbracelet/lipgloss/tree"
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

type TreeBuilder struct {
	visitor TreeVisitor
	depth   int
}

func NewTreeBuilder(visitor TreeVisitor) *TreeBuilder {
	return &TreeBuilder{
		visitor: visitor,
	}
}

func (tb *TreeBuilder) BuildTree(root interface{}) *tree.Tree {
	rootTree := tree.Root(tb.visitor.GetDisplayName(root))
	tb.buildTreeRecursive(rootTree, root, 0)
	return rootTree
}

func (tb *TreeBuilder) buildTreeRecursive(parent *tree.Tree, obj interface{}, depth int) {
	if defaultVisitor, ok := tb.visitor.(*DefaultTreeVisitor); ok {
		if defaultVisitor.MaxDepth > 0 && depth >= defaultVisitor.MaxDepth {
			return
		}
	}

	children := tb.visitor.GetChildren(obj)

	for _, child := range children {
		if tb.visitor.ShouldSkip(child) {
			continue
		}

		var displayName string
		var actualValue interface{}

		if childInfo, ok := child.(map[string]interface{}); ok {
			name := childInfo["name"].(string)
			value := childInfo["value"]

			valueName := tb.visitor.GetDisplayName(value)
			displayName = fmt.Sprintf("%s: %s", name, valueName)
			actualValue = value
		} else {
			displayName = tb.visitor.GetDisplayName(child)
			actualValue = child
		}

		childTree := tree.New().Root(displayName)
		parent.Child(childTree)

		tb.buildTreeRecursive(childTree, actualValue, depth+1)
	}
}

// 편의 함수들

func RenderTree(root interface{}, options ...TreeOption) string {
	visitor := &DefaultTreeVisitor{
		MaxDepth:  10, // 기본 최대 깊이
		SkipEmpty: false,
		ShowTypes: false,
	}

	// 옵션 적용
	for _, option := range options {
		option(visitor)
	}

	builder := NewTreeBuilder(visitor)
	tree := builder.BuildTree(root)

	return tree.String()
}

type TreeOption func(*DefaultTreeVisitor)

func WithMaxDepth(depth int) TreeOption {
	return func(v *DefaultTreeVisitor) {
		v.MaxDepth = depth
	}
}

func WithSkipEmpty(skip bool) TreeOption {
	return func(v *DefaultTreeVisitor) {
		v.SkipEmpty = skip
	}
}

func WithShowTypes(show bool) TreeOption {
	return func(v *DefaultTreeVisitor) {
		v.ShowTypes = show
	}
}
