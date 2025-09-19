package util

func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
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
