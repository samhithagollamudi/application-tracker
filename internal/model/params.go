package model

import "strconv"

// Int16s parses the valid integers from repeated query values (dupes dropped).
func Int16s(vals []string) []int16 {
	var out []int16
	seen := map[int16]bool{}
	for _, v := range vals {
		if n, err := strconv.ParseInt(v, 10, 16); err == nil && n > 0 && !seen[int16(n)] {
			seen[int16(n)] = true
			out = append(out, int16(n))
		}
	}
	return out
}

// Int64s parses the valid positive integers from repeated query values (dupes dropped).
func Int64s(vals []string) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, v := range vals {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// ValidValues keeps only values that are one of opts (dupes dropped).
func ValidValues(opts []Option, vals []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		if !seen[v] && ValidOption(opts, v) {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NonEmpty drops blank values and duplicates (for free-text multi values like countries).
func NonEmpty(vals []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		if v != "" && !seen[v] && len(v) <= 100 {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func In(v string, vals []string) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}
