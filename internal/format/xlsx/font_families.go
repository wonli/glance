package xlsx

import "sort"

// FontFamilies reports the workbook's declared font resources, including styles
// outside the visible window. It does not scan or serialize worksheet values.
func (w *Workbook) FontFamilies() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkOpen(); err != nil {
		return nil, err
	}
	if _, err := w.file.GetStyle(0); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	if w.file.Styles != nil && w.file.Styles.Fonts != nil {
		for _, f := range w.file.Styles.Fonts.Font {
			if f != nil && f.Name != nil && f.Name.Val != nil && *f.Name.Val != "" {
				seen[*f.Name.Val] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}
