package model

import "strconv"

// PageSize is the number of rows per list page.
const PageSize = 50

// Page is one page of a list: Number is 1-based, Total is the number of matching rows.
type Page struct {
	Number int
	Size   int
	Total  int
}

// ParsePage reads the "page" query value (anything invalid means page 1).
func ParsePage(raw string) Page {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		n = 1
	}
	return Page{Number: n, Size: PageSize}
}

// WithTotal sets the total and moves Number back onto the last page if it ran past it.
func (p Page) WithTotal(total int) Page {
	p.Total = total
	if p.Size <= 0 {
		p.Size = PageSize
	}
	if last := p.Pages(); p.Number > last {
		p.Number = last
	}
	if p.Number < 1 {
		p.Number = 1
	}
	return p
}

func (p Page) Pages() int {
	if p.Total == 0 || p.Size <= 0 {
		return 1
	}
	return (p.Total + p.Size - 1) / p.Size
}

func (p Page) Offset() int { return (p.Number - 1) * p.Size }

// From and To are the 1-based row numbers shown ("51–100 of 1,234").
func (p Page) From() int {
	if p.Total == 0 {
		return 0
	}
	return p.Offset() + 1
}

func (p Page) To() int {
	to := p.Offset() + p.Size
	if to > p.Total {
		to = p.Total
	}
	return to
}

// Window returns the page numbers to show as links, with 0 meaning "…".
// e.g. 1 … 4 5 [6] 7 8 … 40
func (p Page) Window() []int {
	last := p.Pages()
	if last <= 9 {
		out := make([]int, last)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	lo, hi := p.Number-2, p.Number+2
	if lo < 2 {
		lo, hi = 2, 6
	}
	if hi > last-1 {
		lo, hi = last-5, last-1
	}
	out := []int{1}
	if lo > 2 {
		out = append(out, 0)
	}
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	if hi < last-1 {
		out = append(out, 0)
	}
	return append(out, last)
}

// Thousands formats 12345 as "12,345".
func Thousands(n int) string {
	s := strconv.Itoa(n)
	neg := n < 0
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
