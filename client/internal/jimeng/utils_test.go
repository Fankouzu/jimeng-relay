package jimeng

import (
	"reflect"
	"testing"
)

func TestToString(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"float64", 123.456, "123"},
		{"int", 789, "789"},
		{"bool", true, "true"},
		{"struct", struct{ A int }{1}, "{1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToString(tt.v); got != tt.want {
				t.Errorf("ToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want int
	}{
		{"nil", nil, 0},
		{"int", 123, 123},
		{"int32", int32(456), 456},
		{"int64", int64(789), 789},
		{"float32", float32(12.34), 12},
		{"float64", 56.78, 56},
		{"string valid", " 100 ", 100},
		{"string invalid", "abc", 0},
		{"bool", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToInt(tt.v); got != tt.want {
				t.Errorf("ToInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want []string
	}{
		{"nil", nil, nil},
		{"string slice", []string{"a", "b"}, []string{"a", "b"}},
		{"interface slice", []interface{}{"c", "d", 123}, []string{"c", "d"}},
		{"invalid type", 123, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToStringSlice(tt.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToStringSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanStringSlice(t *testing.T) {
	tests := []struct {
		name string
		v    []string
		want []string
	}{
		{"empty", []string{}, nil},
		{"nil", nil, nil},
		{"clean", []string{" a ", "", " b ", " "}, []string{"a", "b"}},
		{"no change", []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanStringSlice(tt.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CleanStringSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}
