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

func PtrIfNonNil[T ~[]E, E any](val T) *T {
	if val == nil {
		return nil
	}
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

// ConvertStringSlicePtr converts a slice of strings to a pointer to another
// string-like type. Returns nil if the input slice is nil.
func ConvertStringSlicePtr[T ~string](ss []string) *[]T {
	if ss == nil {
		return nil
	}

	out := make([]T, len(ss))
	for i, s := range ss {
		out[i] = T(s)
	}
	return &out
}

// ConvertSliceToAny converts a slice of some type to slice of any.
// Returns nil if the input slice is nil.
func ConvertSliceToAny[T any](ts []T) []any {
	if ts == nil {
		return nil
	}

	out := make([]any, len(ts))
	for i, t := range ts {
		out[i] = any(t)
	}
	return out

}
