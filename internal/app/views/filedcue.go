package views

import (
	"maps"
	"slices"
	"strconv"

	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin/ntr"
	"github.com/codyconfer/mino/internal/signals"
)

func (k *Kit) withFiledCounts(sections []signals.Section) []signals.Section {
	if len(sections) == 0 || !k.bucketsEnabled() {
		return sections
	}
	var urls []string
	seen := make(map[string]bool)
	for _, s := range sections {
		for _, it := range s.Items {
			if it.URL == "" || seen[it.URL] {
				continue
			}
			seen[it.URL] = true
			urls = append(urls, it.URL)
		}
	}
	if len(urls) == 0 {
		return sections
	}
	home, role := k.ntrHomeRole()
	counts, err := ntr.FiledCounts(home, role, urls)
	if err != nil {
		log.Warnf("buckets: could not read filed counts: %v", err)
		return sections
	}
	if len(counts) == 0 {
		return sections
	}
	return stampFiled(sections, counts)
}

func stampFiled(sections []signals.Section, counts map[string]int) []signals.Section {
	out := slices.Clone(sections)
	for si, s := range out {
		var items []signals.Item
		for ii, it := range s.Items {
			n := counts[it.URL]
			if it.URL == "" || n == 0 {
				continue
			}
			if items == nil {
				items = slices.Clone(s.Items)
			}
			meta := maps.Clone(it.Meta)
			if meta == nil {
				meta = make(map[string]string, 1)
			}
			meta[signals.MetaFiled] = strconv.Itoa(n)
			items[ii].Meta = meta
		}
		if items != nil {
			out[si].Items = items
		}
	}
	return out
}
