package util

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func Ptr[T any](val T) *T {
	return &val
}

func Deref[T any](val *T) T {
	if val == nil {
		var res T
		return res
	}
	return *val
}

func DerefStr[T ~string](val *T) string {
	if val == nil {
		return ""
	}
	return string(*val)
}

func AnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// ConvertStringSlice converts a slice of strings to another string-like type.
// Returns nil if the input slice is nil.
func ConvertStringSlice[T ~string](ss []string) []T {
	if ss == nil {
		return nil
	}

	out := make([]T, len(ss))
	for i, s := range ss {
		out[i] = T(s)
	}
	return out
}
